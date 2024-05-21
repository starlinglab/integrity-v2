package webhook

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	urlpkg "net/url"
	"os"

	"github.com/starlinglab/integrity-v2/config"
)

var client = &http.Client{}

func createNewFileForm(filePath string, metadata map[string]any) (string, io.Reader, error) {
	body := new(bytes.Buffer)
	mp := multipart.NewWriter(body)
	defer mp.Close()
	file, err := os.Open(filePath)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()
	fileInfo, err := file.Stat()
	if err != nil {
		return "", nil, err
	}
	part, err := mp.CreateFormFile("file", fileInfo.Name())
	if err != nil {
		return "", nil, err
	}
	io.Copy(part, file)
	metadataString, err := json.Marshal(metadata)
	if err != nil {
		return "", nil, err
	}
	mp.WriteField("metadata", string(metadataString))
	return mp.FormDataContentType(), body, nil
}

func PostFileToWebHook(filePath string, metadata map[string]any) (map[string]any, error) {
	url, err := urlpkg.Parse(fmt.Sprintf("http://%s/upload", config.GetConfig().Webhook.Host))
	if err != nil {
		return nil, err
	}
	ct, body, err := createNewFileForm(filePath, metadata)
	if err != nil {
		return nil, err
	}
	resp, err := client.Post(url.String(), ct, body)

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
	var value map[string]any
	err = json.NewDecoder(resp.Body).Decode(&value)
	if err != nil {
		return nil, err
	}
	return value, nil
}
