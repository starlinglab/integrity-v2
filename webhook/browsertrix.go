package webhook

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	urlpkg "net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/preprocessor/common"
	wacz "github.com/starlinglab/integrity-v2/preprocessor/wacz"
	"github.com/starlinglab/integrity-v2/util"
)

// Combined fields of various events: crawlFinished, crawlReviewed
type browsertrixEventRequest struct {
	// Common fields
	OrgID  string
	ItemID string
	Event  string

	// crawlFinished fields
	Resources []struct {
		Name        string `json:"name"`
		Path        string `json:"path"`
		Hash        string `json:"hash"`
		Crc32       int    `json:"crc32"`
		Size        int64  `json:"size"`
		CrawlID     string `json:"crawlId"`
		NumReplicas int    `json:"numReplicas"`
		ExpireAt    string `json:"expireAt"`
	} `json:"resources"`

	// crawlReviewed fields
	ReviewStatus      int
	ReviewStatusLabel string
	Description       string
}

type LoginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

var browsertrixJwtMutex = &sync.RWMutex{}
var browsertrixJwtToken string

func getJWTToken() (string, error) {
	browsertrixJwtMutex.RLock()
	if browsertrixJwtToken != "" {
		token, err := jwt.Parse([]byte(browsertrixJwtToken))
		browsertrixJwtMutex.RUnlock()
		if err == nil && token.Expiration().After(time.Now()) {
			return browsertrixJwtToken, nil
		}
	} else {
		browsertrixJwtMutex.RUnlock()
	}
	browsertrixJwtMutex.Lock()
	defer browsertrixJwtMutex.Unlock()
	payload := urlpkg.Values{}
	payload.Set("username", config.GetConfig().Browsertrix.User)
	payload.Set("password", config.GetConfig().Browsertrix.Password)
	req, err := http.NewRequest("POST", "https://app.browsertrix.com/api/auth/jwt/login", strings.NewReader(payload.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to login: %s", resp.Status)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var value LoginResponse
	err = json.Unmarshal(data, &value)
	if err != nil {
		return "", err
	}
	browsertrixJwtToken = value.AccessToken
	return browsertrixJwtToken, nil
}

// https://app.browsertrix.com/api/redoc#tag/crawls/operation/get_crawl_admin_api_orgs_all_crawls__crawl_id__replay_json_get
type CrawlAdminResponse struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Userid      string `json:"userid"`
	UserName    string `json:"userName"`
	Oid         string `json:"oid"`
	Profileid   string `json:"profileid"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Started     string `json:"started"`
	Finished    string `json:"finished"`
	State       string `json:"state"`
	Stats       struct {
		Found int `json:"found"`
		Done  int `json:"done"`
		Size  int `json:"size"`
	} `json:"stats"`
	FileSize      int      `json:"fileSize"`
	FileCount     int      `json:"fileCount"`
	Tags          []string `json:"tags"`
	Errors        []string `json:"errors"`
	CollectionIds []string `json:"collectionIds"`
	Resources     []struct {
		Name        string `json:"name"`
		Path        string `json:"path"`
		Hash        string `json:"hash"`
		Size        int64  `json:"size"`
		CrawlID     string `json:"crawlId"`
		NumReplicas int    `json:"numReplicas"`
		ExpireAt    string `json:"expireAt"`
	} `json:"resources"`
	// other fields are large/unneeded and are omitted
}

func getCrawlInfo(orgId, crawlId string) (*CrawlAdminResponse, error) {
	jwtToken, err := getJWTToken()
	if err != nil {
		return nil, err
	}
	url, err := urlpkg.Parse(
		fmt.Sprintf("https://app.browsertrix.com/api/orgs/%s/crawls/%s/replay.json", orgId, crawlId))
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "Bearer "+jwtToken)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get crawl info: %s", resp.Status)
	}
	defer resp.Body.Close()
	var value CrawlAdminResponse
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(data, &value)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func handleBrowsertrixEvent(w http.ResponseWriter, r *http.Request) {
	webhookSecret := r.URL.Query().Get("s")
	if webhookSecret != config.GetConfig().Browsertrix.WebhookSecret {
		writeJsonResponse(w, http.StatusUnauthorized, map[string]string{"error": "invalid secret"})
		return
	}

	var e browsertrixEventRequest
	defer r.Body.Close()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	err = json.Unmarshal(data, &e)
	if err != nil {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if e.Event != "crawlFinished" && e.Event != "crawlReviewed" {
		log.Printf("browsertrix: received unsupported event %s, ignoring", e.Event)
		w.WriteHeader(http.StatusOK)
		return
	}
	if e.Event == "crawlFinished" && (e.Resources == nil || len(e.Resources) == 0) {
		log.Printf("browsertrix: missing resources, ignoring")
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": "missing resources"})
	}

	crawlInfo, err := getCrawlInfo(e.OrgID, e.ItemID)
	if err != nil {
		log.Printf("browsertrix: failed to get crawl info: %s", err.Error())
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if e.Event == "crawlFinished" && !slices.Contains(crawlInfo.Tags, "auto-accept") {
		// Only accept unreviewed crawls if they (or more likely, the workflow) have been
		// tagged
		log.Println("browsertrix: ignoring finished crawl without auto-accept tag")
		w.WriteHeader(http.StatusOK) // This is normal behaviour
		return
	}

	var projectId string
	for _, tag := range crawlInfo.Tags {
		split := strings.Split(tag, ":")
		if len(split) != 2 {
			continue
		}
		key := split[0]
		value := split[1]
		if key == "project_id" {
			projectId = value
		}
	}
	if projectId == "" {
		log.Printf("browsertrix: missing projectId tag, ignoring")
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": "missing assetOrigin tag"})
		return
	}

	// Check if file is already stored before storing it again
	// Convert SHA-256 hash from Browsertrix to CID, then check the disk
	alreadyDownloaded := false
	matches, _ := aa.IndexMatchQuery("sha256", crawlInfo.Resources[0].Hash, "str")
	if len(matches) > 0 {
		_, err = os.Stat(filepath.Join(config.GetConfig().Dirs.Files, matches[0]))
		if err == nil {
			alreadyDownloaded = true
		}
	}
	if !alreadyDownloaded {
		// Download the WACZ
		waczUrl := crawlInfo.Resources[0].Path
		resp, err := client.Get(waczUrl)
		if err != nil {
			log.Printf("browsertrix: failed to download wacz: %s", err.Error())
			writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			log.Printf("browsertrix: failed to download wacz: %s", resp.Status)
			writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "failed to download wacz"})
			return
		}

		outputDirectory, err := getFileOutputDirectory()
		if err != nil {
			writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		tempFile, err := os.CreateTemp(util.TempDir(), "browsertrix-webhook_")
		if err != nil {
			log.Printf("browsertrix: failed to create temp file: %s", err.Error())
			writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		tempFilePath := tempFile.Name()
		defer tempFile.Close()
		defer os.Remove(tempFilePath)

		cid, fileAttributes, err := getFileAttributesAndWriteToDest(resp.Body, tempFile)
		if err != nil {
			log.Printf("browsertrix: failed to write wacz to temp file: %s", err.Error())
			writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		if fileAttributes["sha256"] != crawlInfo.Resources[0].Hash {
			log.Printf("browsertrix: hash mismatch: %s != %s", fileAttributes["sha256"], crawlInfo.Resources[0].Hash)
			writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "hash mismatch"})
			return
		}

		if fileAttributes["file_size"].(int64) != crawlInfo.Resources[0].Size {
			log.Printf("browsertrix: size mismatch: %d != %d", fileAttributes["file_size"], crawlInfo.Resources[0].Size)
			writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "size mismatch"})
			return
		}

		metadataMap, err := wacz.GetVerifiedMetadata(tempFilePath, nil, common.BrowsertrixSigningDomains())
		if err != nil {
			log.Printf("browsertrix: failed to get metadata: %s", err.Error())
			writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		metadataMap["asset_origin_id"] = crawlInfo.Resources[0].CrawlID
		metadataMap["asset_origin_type"] = []string{"wacz"}
		metadataMap["project_id"] = projectId
		metadataMap["file_name"] = crawlInfo.Resources[0].Name
		metadataMap["crawl_workflow_name"] = crawlInfo.Name
		metadataMap["crawl_workflow_tags"] = crawlInfo.Tags
		metadataMap["crawl_description"] = crawlInfo.Description
		if e.Event == "crawlReviewed" {
			metadataMap["crawl_qa_rating"] = e.ReviewStatusLabel
		}

		err = util.MoveFile(tempFilePath, filepath.Join(outputDirectory, cid))
		if err != nil {
			writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		attributes, err := ParseMapToAttributes(cid, metadataMap, fileAttributes)
		if err != nil {
			log.Println("browsertrix: error parsing attributes:", err)
			writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		err = aa.SetAttestations(cid, true, attributes)
		if err != nil {
			log.Println("browsertrix: error setting attestations:", err)
			writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		log.Printf("browsertrix: processed with CID %s", cid)
	} else {
		// WACZ was already downloaded
		// This must be a "crawlReviewed" event, and the WACZ was already processed during "crawlFinished"
		// So just set any new attributes
		cid := matches[0]
		attrs := []aa.PostKV{
			{Key: "crawl_qa_rating", Value: e.ReviewStatusLabel},
			{Key: "crawl_description", Value: crawlInfo.Description},
		}
		err = aa.SetAttestations(cid, true, attrs)
		if err != nil {
			log.Println("browsertrix: error setting attestations:", err)
			writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		log.Printf("browsertrix: processed QA info for WACZ without re-downloading: %s", cid)
	}

	w.WriteHeader(http.StatusOK)

}
