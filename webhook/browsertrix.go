package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	urlpkg "net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/config"
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
		Size        int    `json:"size"`
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

func getJWTToken() (string, error) {
	b, err := json.Marshal(map[string]string{
		"username": config.GetConfig().Browsertrix.User,
		"password": config.GetConfig().Browsertrix.Password,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("POST", "https://app.browsertrix.com/api/auth/jwt/login", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var value LoginResponse
	err = json.NewDecoder(resp.Body).Decode(&value)
	if err != nil {
		return "", err
	}
	return value.AccessToken, nil
}

type CrawlInfoResponse struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Userid      string    `json:"userid"`
	UserName    string    `json:"userName"`
	Oid         string    `json:"oid"`
	Profileid   string    `json:"profileid"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Started     time.Time `json:"started"`
	Finished    time.Time `json:"finished"`
	State       string    `json:"state"`
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
	defer resp.Body.Close()
	var value CrawlInfoResponse
	err = json.NewDecoder(resp.Body).Decode(&value)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

type CrawlMetadata struct {
	AssetOriginId string
	ProjectId     string
}

func handleBrowsertrixEvent(w http.ResponseWriter, r *http.Request) {
	var e BrowsertrixCrawlFinishedResponse
	err := json.NewDecoder(r.Body).Decode(&e)
	if err != nil {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if e.Event != "crawlFinished" {
		log.Printf("Received event %s, ignoring", e.Event)
		writeJsonResponse(w, http.StatusOK, map[string]string{})
		return
	}
	if e.Resources == nil || len(e.Resources) == 0 {
		log.Printf("Missing resources, ignoring")
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": "missing resources"})
	}

	crawlInfo, err := getCrawlInfo(e.OrgID, e.ItemID)
	if err != nil {
		log.Printf("Failed to get crawl info: %s", err.Error())
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var crawlMetadata CrawlMetadata
	for _, tag := range crawlInfo.Tags {
		split := strings.Split(tag, ":")
		if len(split) != 2 {
			continue
		}
		key := split[0]
		value := split[1]
		if key == "asset_origin_id" {
			crawlMetadata.AssetOriginId = value
		} else if key == "project_id" {
			crawlMetadata.ProjectId = value
		}
	}
	if crawlMetadata.ProjectId == "" {
		log.Printf("Missing projectId tag, ignoring")
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": "missing assetOrigin tag"})
		return
	}

	waczUrl := e.Resources[0].Path
	resp, err := client.Get(waczUrl)
	if err != nil {
		log.Printf("Failed to download wacz: %s", err.Error())
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("Failed to download wacz: %s", resp.Status)
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
		log.Printf("Failed to create temp file: %s", err.Error())
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	tempFilePath := tempFile.Name()
	defer tempFile.Close()
	defer os.Remove(tempFilePath)

	cid, fileAttributes, err := getFileAttributesAndWriteToDest(resp.Body, tempFile)
	if err != nil {
		log.Printf("Failed to write wacz to temp file: %s", err.Error())
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	metadataMap, err := wacz.GetVerifiedMetadata(tempFilePath)
	if err != nil {
		log.Printf("Failed to get metadata: %s", err.Error())
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	metadataMap["asset_origin_id"] = crawlMetadata.AssetOriginId
	metadataMap["asset_origin_type"] = []string{"browsertrix"}
	metadataMap["project_id"] = crawlMetadata.ProjectId

	err = util.MoveFile(tempFilePath, filepath.Join(outputDirectory, cid))
	if err != nil {
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	attributes, err := ParseMapToAttributes(cid, metadataMap, fileAttributes)
	if err != nil {
		log.Println("error parsing attributes:", err)
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	err = aa.SetAttestations(cid, true, attributes)
	if err != nil {
		log.Println("error setting attestations:", err)
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJsonResponse(w, http.StatusOK, map[string]string{"cid": cid})
}
