package c2pa

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const trufoDefaultBaseURL = "https://api.trufo.ai"

// Actions is required by the API even when empty. Each assertion is
// ["name", {params}].
type trufoSignReq struct {
	MediaInput string `json:"media_input"`
	Actions    []any  `json:"actions"`
	Assertions []any  `json:"assertions"`
}

type trufoSignResp struct {
	MediaOutput string `json:"media_output"`
}

// signWithTrufo sends media to Trufo's hosted C2PA signer and returns the
// signed bytes. test selects the test endpoint (outputs not validator-recognized).
func signWithTrufo(baseURL, apiKey string, test bool, media []byte, actions, assertions []any) ([]byte, error) {
	if actions == nil {
		actions = []any{}
	}
	body, err := json.Marshal(trufoSignReq{
		MediaInput: base64.StdEncoding.EncodeToString(media),
		Actions:    actions,
		Assertions: assertions,
	})
	if err != nil {
		return nil, err
	}

	path := "/c2pa/sign"
	if test {
		path = "/test/c2pa/sign"
	}
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trufo sign request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("trufo returned %d: %s", resp.StatusCode, respBody)
	}

	var out trufoSignResp
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("parsing trufo response: %w", err)
	}
	signed, err := base64.StdEncoding.DecodeString(out.MediaOutput)
	if err != nil {
		return nil, fmt.Errorf("decoding trufo output: %w", err)
	}
	return signed, nil
}
