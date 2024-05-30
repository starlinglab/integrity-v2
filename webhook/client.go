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
type PostGenericWebhookOpt struct {
	Source    string // Source is the origin of the asset, which is used to determine the webhook endpoint
	ProjectId string // ProjectId is the project specific ID where the asset belongs
}

type PostGenericWebhookResponse struct {
	Cid   string `json:"cid,omitempty"`
	Error error  `json:"error,omitempty"`
}

// PostFileToWebHook posts a file and its metadata to the webhook server
func PostFileToWebHook(filePath string, metadata map[string]any, opts PostGenericWebhookOpt) (*PostGenericWebhookResponse, error) {
	sourcePath := opts.Source
	if sourcePath == "" {
		sourcePath = "generic"
	}
	url, err := urlpkg.Parse(fmt.Sprintf("http://%s/%s", config.GetConfig().Webhook.Host, sourcePath))
	if err != nil {
		return nil, err
	}
	q := url.Query()
	if opts.ProjectId != "" {
		q.Add("project_id", opts.ProjectId)
	}
	pr, pw := io.Pipe()
	mp := multipart.NewWriter(pw)

	go func() {
		metadataString, err := json.Marshal(metadata)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		err = mp.WriteField("metadata", string(metadataString))
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		file, err := os.Open(filePath)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		defer file.Close()
		part, err := mp.CreateFormFile("file", filepath.Base(filePath))
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		_, err = io.Copy(part, file)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		pw.CloseWithError(mp.Close())
	}()
	req, err := http.NewRequest("POST", url.String(), pr)

	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mp.FormDataContentType())
	jwt := config.GetConfig().Webhook.Jwt
	if jwt != "" {
		req.Header.Add("Authorization", "Bearer "+jwt)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 400 {
		return nil, errors.New("bad request")
	}
	if resp.StatusCode == 404 {
		return nil, errors.New("bad request")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bad status code in response: %d", resp.StatusCode)
	}
	value := PostGenericWebhookResponse{}
	err = json.NewDecoder(resp.Body).Decode(&value)
	if err != nil {
		return nil, err
	}
	return &value, nil
}
