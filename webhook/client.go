package webhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	urlpkg "net/url"
	"os"
	"path/filepath"

	"github.com/starlinglab/integrity-v2/config"
)

var client = &http.Client{}

// PostWebhookOpt is the options for posting a file to the webhook server
// Source is the origin of the asset, which is used to determine the webhook endpoint
// Jwt is the JWT token to authenticate the request
type PostWebhookOpt struct {
	Source string
}

// PostFileToWebHook posts a file and its metadata to the webhook server
func PostFileToWebHook(filePath string, metadata map[string]any, opts PostWebhookOpt) (map[string]any, error) {
	sourcePath := opts.Source
	if sourcePath == "" {
		sourcePath = "upload"
	}
	url, err := urlpkg.Parse(fmt.Sprintf("http://%s/%s", config.GetConfig().Webhook.Host, sourcePath))
	if err != nil {
		return nil, err
	}
	pr, pw := io.Pipe()
	mp := multipart.NewWriter(pw)

	er := make(chan error)
	go func() {
		metadataString, err := json.Marshal(metadata)
		if err != nil {
			er <- err
			return
		}
		err = mp.WriteField("metadata", string(metadataString))
		if err != nil {
			er <- err
			return
		}
		file, err := os.Open(filePath)
		if err != nil {
			er <- err
			return
		}
		defer file.Close()
		part, err := mp.CreateFormFile("file", filepath.Base(filePath))
		if err != nil {
			er <- err
			return
		}
		_, err = io.Copy(part, file)
		if err != nil {
			er <- err
			return
		}
		err = mp.Close()
		if err != nil {
			er <- err
			return
		}
	}()
	req, err := http.NewRequest("POST", url.String(), pr)

	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mp.FormDataContentType())
	Jwt := config.GetConfig().Webhook.Jwt
	if Jwt != "" {
		req.Header.Add("Authorization", "Bearer "+Jwt)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	err = <-er
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 400 {
		return nil, errors.New("bad request")
	}
	if resp.StatusCode == 404 {
		return nil, errors.New("bad request")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bad status code in response: %d", resp.StatusCode)
	}
	var value map[string]any
	err = json.NewDecoder(resp.Body).Decode(&value)
	if err != nil {
		return nil, err
	}
	return value, nil
}
