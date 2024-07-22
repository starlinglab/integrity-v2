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

type BrowsertrixCrawlFinishedResponse struct {
	OrgID     string `json:"orgId"`
	ItemID    string `json:"itemId"`
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
	State string `json:"state"`
	Event string `json:"event"`
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

type CrawlInfoResponse struct {
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
	// other fields are large and are omited
}

func getCrawlInfo(orgId, crawlId string) (*CrawlInfoResponse, error) {
	jwtToken, err := getJWTToken()
	if err != nil {
		return nil, err
	}
	url, err := urlpkg.Parse(fmt.Sprintf("https://app.browsertrix.com/api/orgs/%s/crawls/%s", orgId, crawlId))
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
	var value CrawlInfoResponse
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

	var e BrowsertrixCrawlFinishedResponse
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
	if e.Event != "crawlFinished" {
		log.Printf("browsertrix: received event %s, ignoring", e.Event)
		w.WriteHeader(http.StatusOK)
		return
	}
	if e.Resources == nil || len(e.Resources) == 0 {
		log.Printf("browsertrix: missing resources, ignoring")
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": "missing resources"})
	}

	crawlInfo, err := getCrawlInfo(e.OrgID, e.ItemID)
	if err != nil {
		log.Printf("browsertrix: failed to get crawl info: %s", err.Error())
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
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

	waczUrl := e.Resources[0].Path
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

	if fileAttributes["sha256"] != e.Resources[0].Hash {
		log.Printf("browsertrix: hash mismatch: %s != %s", fileAttributes["sha256"], e.Resources[0].Hash)
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "hash mismatch"})
		return
	}

	if fileAttributes["file_size"].(int64) != e.Resources[0].Size {
		log.Printf("browsertrix: size mismatch: %d != %d", fileAttributes["file_size"], e.Resources[0].Size)
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "size mismatch"})
		return
	}

	metadataMap, err := wacz.GetVerifiedMetadata(tempFilePath, nil, common.BrowsertrixSigningDomains)
	if err != nil {
		log.Printf("browsertrix: failed to get metadata: %s", err.Error())
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	metadataMap["asset_origin_id"] = e.Resources[0].CrawlID
	metadataMap["asset_origin_type"] = []string{"wacz"}
	metadataMap["project_id"] = projectId
	metadataMap["file_name"] = e.Resources[0].Name
	metadataMap["crawl_workflow_name"] = crawlInfo.Name
	metadataMap["crawl_workflow_description"] = crawlInfo.Description
	metadataMap["crawl_workflow_tags"] = crawlInfo.Tags

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

	w.WriteHeader(http.StatusOK)

	log.Printf("browsertrix: processed with CID %s", cid)
}
