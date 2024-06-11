package webhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	urlpkg "net/url"
	"strings"

	"github.com/fxamacker/cbor/v2"
	"github.com/starlinglab/integrity-v2/config"
)

var client = &http.Client{}

// PostWebhookOpt is the options for posting a file to the webhook server
type PostGenericWebhookOpt struct {
	Source    string // Source is the origin of the asset, which is used to determine the webhook endpoint
	ProjectId string // ProjectId is the project specific ID where the asset belongs
	Format    string // Format is "json" or "cbor"
}

type PostGenericWebhookResponse struct {
	Cid   string `json:"cid,omitempty"`
	Error error  `json:"error,omitempty"`
}

// createFormFieldWithContentType is a modified multipart.Writer.CreateFormField
// which allow setting of content-type
func createFormFieldWithContentType(w *multipart.Writer, fieldName string, contentType string) (io.Writer, error) {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="%s"`, strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(fieldName)))
	h.Set("Content-Type", contentType)
	return w.CreatePart(h)
}

// PostFileToWebHook posts a file and its metadata to the webhook server
func PostFileToWebHook(file io.Reader, metadata map[string]any, opts PostGenericWebhookOpt) (*PostGenericWebhookResponse, error) {
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

	metadataFormatType := "application/json"
	if opts.Format == "cbor" {
		metadataFormatType = "application/cbor"
	}

	go func() {
		metadataPart, err := createFormFieldWithContentType(mp, "metadata", metadataFormatType)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		if metadataFormatType == "application/cbor" {
			err = cbor.NewEncoder(metadataPart).Encode(metadata)

		} else {
			err = json.NewEncoder(metadataPart).Encode(metadata)
		}
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		filePart, err := mp.CreateFormFile("file", "")
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		_, err = io.Copy(filePart, file)
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
