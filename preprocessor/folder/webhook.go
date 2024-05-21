package preprocessor_folder

import (
	"fmt"

	"github.com/starlinglab/integrity-v2/webhook"
)

func postFileMetadataToWebHook(filePath string, metadata map[string]any) (string, error) {
	body, err := webhook.PostFileToWebHook(filePath, metadata)
	if err != nil {
		return "", err
	}
	cid, ok := body["cid"].(string)
	if !ok {
		return "", fmt.Errorf("error parsing cid from webhook response")
	}
	return cid, nil
}
