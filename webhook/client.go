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

// PostFileToWebHook posts a file and its metadata to the webhook server
func PostFileToWebHook(filePath string, metadata map[string]any) (map[string]any, error) {
	url, err := urlpkg.Parse(fmt.Sprintf("http://%s/upload", config.GetConfig().Webhook.Host))
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
	resp, err := client.Post(url.String(), mp.FormDataContentType(), pr)
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
