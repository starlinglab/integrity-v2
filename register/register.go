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
	"slices"
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

// chainCardano is the --on value (and the recorded aaRegistration.Chain) for the Cardano path. It
// ties the write side (Run) to the read-side match in matchCardanoRegistration.
const chainCardano = "cardano"

func Run(args []string) error {
	fs := flag.NewFlagSet("register", flag.ContinueOnError)
	fs.StringVar(&chain, "on", "", "Chain/network to register asset on (numbers,avalanche,ethereum,polygon,cardano)")
	fs.StringVar(&include, "include", "", "Comma-separated list of attributes to register (optional)")
	fs.BoolVar(&testnet, "testnet", false, "Register on a test network (if supported); for cardano this selects the preview testnet, and its absence selects mainnet")
	fs.BoolVar(&dryRun, "dry-run", false, "show registration info without actually sending it")

	err := fs.Parse(args)
	if err != nil {
		// Error is already printed
		os.Exit(1)
	}

	// Validate input
	if chain == "" {
		fs.PrintDefaults()
		return fmt.Errorf("\nprovide chain/network with --on: numbers,avalanche,ethereum,polygon,cardano")
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("provide a single CID to work with")
	}

	cid := fs.Arg(0)

	// Chains registered through the Numbers Protocol API; everything else
	// (currently just cardano) has its own registration path.
	numbersChains := []string{"numbers", "avalanche", "ethereum", "polygon"}
	isNumbers := slices.Contains(numbersChains, chain)
	if !isNumbers && chain != chainCardano {
		return fmt.Errorf("invalid chain name")
	}

	// The cardano network name is needed by both the idempotency precheck and the pending-record
	// cleanup below; derive it once.
	var cardanoNetName string
	if chain == chainCardano {
		cardanoNetName = cardanoNetworkFor(testnet).name
	}

	// Idempotency guard: if this CID is already registered on the selected cardano network, do not
	// build or submit anything — re-running is a no-op success. mainnet and preview are distinct, so
	// registering on one after the other is still allowed. Skipped under --dry-run, which is meant to
	// show the would-be payload rather than short-circuit.
	if chain == chainCardano && !dryRun {
		existing, err := existingCardanoRegistration(cid, cardanoNetName)
		if err != nil {
			// Fail closed on a read error (transient AA outage, auth): refusing to proceed when we
			// cannot verify a prior registration is safer than risking a duplicate fee-paying tx. A
			// retry once AuthAttr is reachable succeeds.
			return err
		}
		if existing != nil {
			fmt.Printf("Already registered on cardano (%s): tx %s\n", existing.CardanoChain, existing.TxHash)
			return nil
		}
	}

	requestData := map[string]any{
		"assetCid":     cid,
		"assetCreator": "Starling Lab",
		"testnet":      testnet,
	}

	// The Numbers Protocol API selects the target chain via nftChainID.
	// https://docs.numbersprotocol.io/developers/commit-asset-history/support-status/
	if isNumbers {
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
		requestData["nftChainID"] = chainID
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

	requestBytes, err := json.Marshal(requestData)
	if err != nil {
		return fmt.Errorf("failed to marshal request JSON: %w", err)
	}

	var chainData any
	if isNumbers {
		chainData, err = numbersRegister(requestBytes)
	} else {
		chainData, err = cardanoRegister(cid, string(requestBytes), testnet)
	}
	if err != nil {
		return err
	}

	err = aa.AppendAttestation(cid, "registrations", aaRegistration{
		Chain: chain,
		Attrs: attrNames,
		Data:  chainData,
	})
	if err != nil {
		return fmt.Errorf("error logging registration to AuthAttr: %w", err)
	}

	// The registration is now durably recorded, so the crash-safe pending record can be removed.
	// Clearing only after the append means a crash before this point leaves the pending record in
	// place, letting a re-run resume the existing tx rather than submit a duplicate.
	if chain == chainCardano {
		if err := clearPendingCardano(conf, cardanoNetName, cid); err != nil {
			fmt.Printf("warning: could not clear pending cardano record: %v\n", err)
		}
	}

	fmt.Println("Success.")
	fmt.Println("Logged registration to AuthAttr under the attribute 'registrations'.")
	return nil
}

func numbersRegister(requestBytes []byte) (*numbersCommitResp, error) {
	// Docs: https://docs.numbersprotocol.io/developers/commit-asset-history/commit-via-api

	if config.GetConfig().Numbers.Token == "" {
		return nil, fmt.Errorf("numbers authentication token not set in config file")
	}

	req, err := http.NewRequest(
		"POST",
		"https://us-central1-numbers-protocol-api.cloudfunctions.net/nit-commit-to-jade",
		bytes.NewReader(requestBytes),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "token "+config.GetConfig().Numbers.Token)
	req.Header.Add("Content-Type", "application/json")
	fmt.Println("Registering...")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error with register API call: %w", err)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if err != nil {
		return nil, fmt.Errorf("error reading API response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("register server returned status code %d and body: %s",
			resp.StatusCode, body)
	}

	if testnet {
		fmt.Printf("\n%s\n\nTestnet registration not logged in AuthAttr\n", body)
		return nil, nil
	}

	var txData numbersCommitResp
	err = json.Unmarshal(body, &txData)
	if err != nil {
		return nil, fmt.Errorf("error parsing API response: %w", err)
	}
	return &txData, nil
}

type numbersCommitResp struct {
	TxHash       string `json:"txHash"`
	AssetCid     string `json:"assetCid"`
	AssetTreeCid string `json:"assetTreeCid"`
	OrderId      string `json:"order_id"`
}

type aaRegistration struct {
	Chain string   `cbor:"chain"`
	Attrs []string `cbor:"attrs"`
	Data  any      `cbor:"data"`
}

func getAttValue(cid string, attr string) (any, error) {
	att, err := aa.GetAttestation(cid, attr, aa.GetAttOpts{})
	if err != nil {
		return nil, fmt.Errorf("error getting attestation '%s': %w", attr, err)
	}
	return att.Attestation.Value, nil
}
