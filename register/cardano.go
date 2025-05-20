package register

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/util"
)

// Hardcode use of "preview" testnet for now
const (
	cardanoNetworkId = "2"
	blockfrostApi    = "https://cardano-preview.blockfrost.io/api/v0/"
	cardanoMsgNumber = 674     // Generic transaction message
	cardanoFee       = 500_000 // XXX: overestimate fee for simplicity
)

type cardanoChainData struct {
	CardanoChain string `json:"cardano_chain"`
	TxHash       string `json:"tx_hash"`
}

func cardanoRegister(msg string) (*cardanoChainData, error) {
	conf := config.GetConfig()

	if conf.Bins.CardanoCli == "" {
		return nil, fmt.Errorf("no cardano-cli path set in the config")
	}
	if conf.Dirs.Cardano == "" {
		return nil, fmt.Errorf("cardano dirs are not set in config")
	}

	// Generate wallet and address if needed
	ok1, err := util.FileExists(filepath.Join(conf.Dirs.Cardano, "payment.vkey"))
	if err != nil {
		return nil, err
	}
	ok2, err := util.FileExists(filepath.Join(conf.Dirs.Cardano, "payment.skey"))
	if err != nil {
		return nil, err
	}
	if !(ok1 && ok2) {
		fmt.Println("Generating key")
		err := runCardanoCmd(
			"address",
			"key-gen",
			"--verification-key-file",
			filepath.Join(conf.Dirs.Cardano, "payment.vkey"),
			"--signing-key-file",
			filepath.Join(conf.Dirs.Cardano, "payment.skey"),
		)
		if err != nil {
			return nil, err
		}
		fmt.Println("Building address")
		err = runCardanoCmd(
			"address",
			"build",
			"--payment-verification-key-file",
			filepath.Join(conf.Dirs.Cardano, "payment.vkey"),
			"--out-file",
			filepath.Join(conf.Dirs.Cardano, "paymentNoStake.addr"),
			"--testnet-magic",
			cardanoNetworkId,
		)
		if err != nil {
			return nil, err
		}
	}

	// Get UTXOs
	fmt.Println("Getting UXTOs")
	addr, err := os.ReadFile(filepath.Join(conf.Dirs.Cardano, "paymentNoStake.addr"))
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("GET", blockfrostApi+"addresses/"+string(addr)+"/utxos", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("project_id", conf.Cardano.BlockfrostApiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil,
			fmt.Errorf("address (%s) has no funds, go to the faucet: https://docs.cardano.org/cardano-testnets/tools/faucet",
				addr)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	var uxtos uxtoResp
	if err := json.Unmarshal(body, &uxtos); err != nil {
		return nil, err
	}

	// Choose UXTO(s) to use
	// TODO: don't just use the first one
	quantity, err := strconv.Atoi(uxtos[0].Amount[0].Quantity)
	if err != nil {
		return nil, fmt.Errorf("blockfrost uxto quantity is unparseable: %v", err)
	}
	if quantity < cardanoFee {
		return nil, fmt.Errorf("add more funds, go to the faucet: https://docs.cardano.org/cardano-testnets/tools/faucet")
	}

	// Save message
	b, err := json.Marshal(map[string][]string{strconv.Itoa(cardanoMsgNumber): cardanoSplitStr(msg)})
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(conf.Dirs.Cardano, "tx_message.json"), b, 0644); err != nil {
		return nil, err
	}

	// Build transaction
	fmt.Println("Building transaction")
	err = runCardanoCmd(
		"conway",
		"transaction",
		"build-raw",
		"--tx-in",
		uxtos[0].TxHash+"#"+strconv.Itoa(uxtos[0].TxIndex), // TODO: bad default
		"--tx-out",
		string(addr)+"+"+strconv.Itoa(quantity-cardanoFee),
		"--fee",
		strconv.Itoa(cardanoFee), // XXX: overestimated static fee
		"--metadata-json-file",
		filepath.Join(conf.Dirs.Cardano, "tx_message.json"),
		"--out-file",
		filepath.Join(conf.Dirs.Cardano, "tx_new_message.draft"),
	)
	if err != nil {
		return nil, err
	}

	// Sign transaction
	fmt.Println("Signing transaction")
	err = runCardanoCmd(
		"conway",
		"transaction",
		"sign",
		"--tx-body-file",
		filepath.Join(conf.Dirs.Cardano, "tx_new_message.draft"),
		"--signing-key-file",
		filepath.Join(conf.Dirs.Cardano, "payment.skey"),
		"--testnet-magic",
		cardanoNetworkId,
		"--out-file",
		filepath.Join(conf.Dirs.Cardano, "tx_new_message.signed"),
	)
	if err != nil {
		return nil, err
	}

	// Extract CBOR from signed transaction
	b, err = os.ReadFile(filepath.Join(conf.Dirs.Cardano, "tx_new_message.signed"))
	if err != nil {
		return nil, err
	}
	var data map[string]string
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	txCbor, err := hex.DecodeString(data["cborHex"])
	if err != nil {
		return nil, err
	}

	// Submit transaction to Blockfrost
	fmt.Println("Submitting transaction")
	req, err = http.NewRequest("POST", blockfrostApi+"tx/submit", bytes.NewReader(txCbor))
	if err != nil {
		return nil, err
	}
	req.Header.Add("project_id", conf.Cardano.BlockfrostApiKey)
	req.Header.Add("Content-Type", "application/cbor")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("blockfrost tx/submit return status code %d", resp.StatusCode)
	}

	// We get back the transaction hash
	// For now let's assume the transaction gets accepted and not get other info
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	var txHash string
	if err := json.Unmarshal(body, &txHash); err != nil {
		return nil, err
	}

	return &cardanoChainData{
		CardanoChain: "preview", // The hardcoded chain
		TxHash:       txHash,
	}, nil
}

type uxtoResp []struct {
	TxHash  string `json:"tx_hash"`
	TxIndex int    `json:"tx_index"`
	Amount  []struct {
		Unit     string `json:"unit"`
		Quantity string `json:"quantity"`
	} `json:"amount"`
}

func runCardanoCmd(args ...string) error {
	fmt.Println(args)
	conf := config.GetConfig()
	cmd := exec.Command(conf.Bins.CardanoCli, args...)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("cardano-cli not found at configured path, may not be installed: %s",
				conf.Bins.CardanoCli)
		}
		return err
	}
	return nil
}

// cardanoSplitStr splits up a a string so it can be used as Cardano metadata
func cardanoSplitStr(msg string) []string {
	// String are limited to 64 bytes
	// https://developers.cardano.org/docs/transaction-metadata/
	ss := make([]string, (len(msg)+64-1)/64) // ceiling division
	for i := range ss {
		high := i*64 + 64
		if high >= len(msg) {
			high = len(msg) - 1
		}
		ss[i] = msg[i*64 : high]
	}
	return ss
}
