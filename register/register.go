package register

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/config"
)

var (
	chain   string
	include string
	testnet bool
	dryRun  bool
)

func Run(args []string) error {
	fs := flag.NewFlagSet("register", flag.ContinueOnError)
	fs.StringVar(&chain, "on", "", "Chain/network to register asset on (numbers,avalanche,ethereum,polygon)")
	fs.StringVar(&include, "include", "", "Comma-separated list of attributes to register (optional)")
	fs.BoolVar(&testnet, "testnet", false, "Register on a test network (if supported)")
	fs.BoolVar(&dryRun, "dry-run", false, "show registration info without actually sending it")

	err := fs.Parse(args)
	if err != nil {
		// Error is already printed
		os.Exit(1)
	}

	// Validate input
	if chain == "" {
		fs.PrintDefaults()
		return fmt.Errorf("\nprovide chain/network with --on: numbers,avalanche,ethereum,polygon")
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("provide a single CID to work with")
	}

	cid := fs.Arg(0)

	// https://docs.numbersprotocol.io/developers/commit-asset-history/support-status/
	var chainID int
	switch chain {
	case "numbers":
		chainID = 10507
	case "avalanche":
		chainID = 43114
	case "ethereum":
		chainID = 1
	case "polygon":
		chainID = 137
	}
	if chainID == 0 {
		return fmt.Errorf("invalid chain name")
	}

	// Currently only one registration API is supported: Numbers Protocol
	// Docs: https://docs.numbersprotocol.io/developers/commit-asset-history/commit-via-api

	requestData := map[string]any{
		"assetCid":     cid,
		"assetCreator": "Starling Lab",
		"testnet":      testnet,
		"nftChainID":   chainID,
	}

	var attrNames []string
	if include != "" {
		attrNames = strings.Split(include, ",")
		metadata := make(map[string]any)
		for _, attr := range attrNames {
			var err error
			metadata[attr], err = getAttValue(cid, attr)
			if err != nil {
				return err
			}
		}
		requestData["custom"] = metadata
	}

	// Required fields

	requestData["encodingFormat"], err = getAttValue(cid, "media_type")
	if err != nil {
		return err
	}
	requestData["assetSha256"], err = getAttValue(cid, "sha256")
	if err != nil {
		return err
	}

	tmp, err := getAttValue(cid, "time_created")
	if err != nil {
		return err
	}
	timeCreated, ok := tmp.(string)
	if !ok {
		return fmt.Errorf("schema error: time_created is not a string")
	}
	tmp2, err := time.Parse(time.RFC3339, timeCreated)
	if err != nil {
		return fmt.Errorf("schema error: time_created is not RFC3339: %w", err)
	}
	requestData["assetTimestampCreated"] = tmp2.Unix()

	// Optional fields

	conf := config.GetConfig()
	if conf.Numbers.NftContractAddress != "" {
		requestData["nftContractAddress"] = conf.Numbers.NftContractAddress
	}
	requestData["abstract"], err = getAttValue(cid, "description")
	if err != nil && !errors.Is(err, aa.ErrNotFound) {
		return err
	}
	requestData["headline"], err = getAttValue(cid, "name")
	if err != nil && !errors.Is(err, aa.ErrNotFound) {
		return err
	}

	if dryRun {
		j, err := json.MarshalIndent(requestData, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal request JSON: %w", err)
		}
		os.Stdout.Write(j)
		fmt.Println()
		return nil
	}

	if conf.Numbers.Token == "" {
		return fmt.Errorf("numbers authentication token not set in config file")
	}

	requestBytes, err := json.Marshal(requestData)
	if err != nil {
		return fmt.Errorf("failed to marshal request JSON: %w", err)
	}

	req, err := http.NewRequest(
		"POST",
		"https://us-central1-numbers-protocol-api.cloudfunctions.net/nit-commit-to-jade",
		bytes.NewReader(requestBytes),
	)
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", "token "+conf.Numbers.Token)
	req.Header.Add("Content-Type", "application/json")
	fmt.Println("Registering...")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("error with register API call: %w", err)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if err != nil {
		return fmt.Errorf("error reading API response: %w", err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("register server returned status code %d and body: %s",
			resp.StatusCode, body)
	}

	if testnet {
		fmt.Printf("\n%s\n\nTestnet registration not logged in AuthAttr\n", body)
		return nil
	}

	var txData numbersCommitResp
	err = json.Unmarshal(body, &txData)
	if err != nil {
		return fmt.Errorf("error parsing API response: %w", err)
	}

	err = aa.AppendAttestation(cid, "registrations", aaRegistration{
		Chain: chain,
		Attrs: attrNames,
		Data:  &txData,
	})
	if err != nil {
		return fmt.Errorf("error logging registration to AuthAttr: %w", err)
	}

	fmt.Println("Success.")
	fmt.Println("Logged registration to AuthAttr under the attribute 'registrations'.")
	return nil
}

type numbersCommitResp struct {
	TxHash       string `json:"txHash"`
	AssetCid     string `json:"assetCid"`
	AssetTreeCid string `json:"assetTreeCid"`
	OrderId      string `json:"order_id"`
}

type aaRegistration struct {
	Chain string             `cbor:"chain"`
	Attrs []string           `cbor:"attrs"`
	Data  *numbersCommitResp `cbor:"data"`
}

func getAttValue(cid string, attr string) (any, error) {
	att, err := aa.GetAttestation(cid, attr, aa.GetAttOpts{})
	if err != nil {
		return nil, fmt.Errorf("error getting attestation '%s': %w", attr, err)
	}
	return att.Attestation.Value, nil
}
