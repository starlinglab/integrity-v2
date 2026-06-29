package register

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/util"
)

// Hardcode use of "preview" testnet for now
const (
	cardanoNetworkId = "2"
	blockfrostApi    = "https://cardano-preview.blockfrost.io/api/v0/"
	cardanoMsgNumber = 674 // Generic transaction message

	// cardanoFeePlaceholder is a stand-in fee used only to build and sign the tx once so
	// we can measure its on-chain byte size. Its CBOR integer width (5 bytes, since it
	// exceeds 65535) matches that of the real fee, so the measured size equals the final
	// tx's size.
	cardanoFeePlaceholder = 200_000
	// cardanoFeeMargin is a small lovelace cushion added above the computed minimum fee.
	// The minimum fee is a floor, so paying slightly over is always accepted; this absorbs
	// any ±1-2 byte size drift between the measuring build and the final build.
	cardanoFeeMargin = 1_000

	cardanoPollInterval = 5 * time.Second  // gap between confirmation checks
	cardanoPollTimeout  = 10 * time.Minute // give up (fail registration) after this

	cardanoFaucetErr = "add more funds, go to the faucet: https://docs.cardano.org/cardano-testnets/tools/faucet"
)

type cardanoChainData struct {
	CardanoChain string `json:"cardano_chain"`
	TxHash       string `json:"tx_hash"`
	BlockHeight  int64  `json:"block_height"`
	BlockTime    int64  `json:"block_time"` // unix seconds, from Blockfrost block_time
	Status       string `json:"status"`     // "confirmed" (only confirmed txs are persisted)
}

// cardanoTxResp is the subset of Blockfrost's GET /txs/{hash} response we need.
type cardanoTxResp struct {
	BlockHeight int64 `json:"block_height"`
	BlockTime   int64 `json:"block_time"`
}

// cardanoProtocolParams is the subset of Blockfrost's GET /epochs/latest/parameters
// response we need: the linear fee coefficients (fee = min_fee_b + min_fee_a*txSize).
type cardanoProtocolParams struct {
	MinFeeA int `json:"min_fee_a"`
	MinFeeB int `json:"min_fee_b"`
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

	// Choose UTXO(s) to spend. We need at least enough lovelace to build the measuring
	// transaction; the real fee (computed below) is smaller, so cardanoFeePlaceholder is a
	// conservative lower bound. Any native assets carried by the selected UTXOs are returned
	// in the change output (see buildAndSignCardanoTx) so the transaction preserves value.
	parsed, err := parseCardanoUTXOs(uxtos)
	if err != nil {
		return nil, err
	}
	txIns, quantity, assets, err := selectCardanoUTXOs(parsed, cardanoFeePlaceholder)
	if err != nil {
		return nil, err
	}

	// Fetch the current protocol fee parameters so we can size the fee to the actual
	// transaction instead of overpaying a static amount.
	pp, err := getCardanoProtocolParams(context.Background(), conf.Cardano.BlockfrostApiKey)
	if err != nil {
		return nil, err
	}

	// Save message
	b, err := json.Marshal(map[string][]string{strconv.Itoa(cardanoMsgNumber): cardanoSplitStr(msg)})
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(conf.Dirs.Cardano, "tx_message.json"), b, 0644); err != nil {
		return nil, err
	}

	// Pass 1: build and sign with a placeholder fee solely to measure the signed tx size.
	measured, err := buildAndSignCardanoTx(conf, string(addr), txIns, cardanoFeePlaceholder, quantity-cardanoFeePlaceholder, assets)
	if err != nil {
		return nil, err
	}

	// Compute the real fee from the protocol params and the measured size.
	fee := cardanoMinFee(pp.MinFeeA, pp.MinFeeB, len(measured))
	if quantity <= fee {
		return nil, errors.New(cardanoFaucetErr)
	}

	// Pass 2: rebuild and re-sign with the computed fee and matching change output.
	txCbor, err := buildAndSignCardanoTx(conf, string(addr), txIns, fee, quantity-fee, assets)
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

	// Poll until the transaction is included in a block, recording where it landed.
	// A 200 from tx/submit only means the tx was accepted into the mempool.
	fmt.Println("Waiting for on-chain confirmation")
	tx, err := pollCardanoConfirmation(txHash, conf.Cardano.BlockfrostApiKey)
	if err != nil {
		return nil, err
	}

	return &cardanoChainData{
		CardanoChain: "preview", // The hardcoded chain
		TxHash:       txHash,
		BlockHeight:  tx.BlockHeight,
		BlockTime:    tx.BlockTime,
		Status:       "confirmed",
	}, nil
}

// buildAndSignCardanoTx builds a raw transaction spending every txIn in txIns back to addr
// with the given fee and change lovelace (change = total input lovelace - fee), returning any
// native assets carried by the inputs to the same change output so the transaction preserves
// value. It attaches the metadata file written by the caller, signs the tx, and returns the
// signed transaction's raw CBOR bytes. len(cbor) is the transaction's on-chain size, used to
// compute the fee.
func buildAndSignCardanoTx(conf *config.Config, addr string, txIns []string, fee, change int, assets map[string]int) ([]byte, error) {
	fmt.Println("Building transaction")
	args := []string{"conway", "transaction", "build-raw"}
	for _, txIn := range txIns {
		args = append(args, "--tx-in", txIn)
	}
	args = append(args,
		"--tx-out",
		addr+"+"+cardanoTxOutValue(change, assets),
		"--fee",
		strconv.Itoa(fee),
		"--metadata-json-file",
		filepath.Join(conf.Dirs.Cardano, "tx_message.json"),
		"--out-file",
		filepath.Join(conf.Dirs.Cardano, "tx_new_message.draft"),
	)
	err := runCardanoCmd(args...)
	if err != nil {
		return nil, err
	}

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

	b, err := os.ReadFile(filepath.Join(conf.Dirs.Cardano, "tx_new_message.signed"))
	if err != nil {
		return nil, err
	}
	var data map[string]string
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	return hex.DecodeString(data["cborHex"])
}

// cardanoMinFee returns the minimum fee (in lovelace) for a transaction of txSize bytes that
// has no Plutus scripts: the Cardano ledger defines this as the linear function
// minFeeB + minFeeA*size, to which we add cardanoFeeMargin as a small safety cushion.
func cardanoMinFee(minFeeA, minFeeB, txSize int) int {
	return minFeeB + minFeeA*txSize + cardanoFeeMargin
}

// getCardanoProtocolParams fetches the current epoch's protocol parameters from Blockfrost,
// returning the linear fee coefficients used to size transaction fees.
func getCardanoProtocolParams(ctx context.Context, apiKey string) (*cardanoProtocolParams, error) {
	resp, err := blockfrostGet(ctx, "epochs/latest/parameters", apiKey)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("blockfrost epochs/latest/parameters returned status code %d", resp.StatusCode)
	}

	var pp cardanoProtocolParams
	if err := json.NewDecoder(resp.Body).Decode(&pp); err != nil {
		return nil, err
	}
	if pp.MinFeeA <= 0 || pp.MinFeeB <= 0 {
		return nil, fmt.Errorf("blockfrost returned non-positive fee params: min_fee_a=%d min_fee_b=%d",
			pp.MinFeeA, pp.MinFeeB)
	}
	return &pp, nil
}

// pollCardanoConfirmation polls Blockfrost GET /txs/{hash} until txHash is included
// in a block, returning its block height and time. Blockfrost returns 404 while the
// tx is still pending. It fails if the tx is not confirmed within cardanoPollTimeout.
func pollCardanoConfirmation(txHash, apiKey string) (*cardanoTxResp, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cardanoPollTimeout)
	defer cancel()

	for {
		tx, err := getCardanoTx(ctx, txHash, apiKey)
		if err != nil {
			return nil, err
		}
		if tx != nil {
			return tx, nil
		}

		// Not on-chain yet; wait before the next check unless the deadline elapses.
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("cardano tx %s not confirmed within %s", txHash, cardanoPollTimeout)
		case <-time.After(cardanoPollInterval):
		}
	}
}

// blockfrostGet issues an authenticated GET against the Blockfrost API. The caller
// owns the returned response (must close its body) and handles the status code.
func blockfrostGet(ctx context.Context, path, apiKey string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", blockfrostApi+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("project_id", apiKey)
	return http.DefaultClient.Do(req)
}

// getCardanoTx fetches a single tx from Blockfrost. It returns (nil, nil) when the tx
// is not yet on-chain (HTTP 404), so the caller can keep polling.
func getCardanoTx(ctx context.Context, txHash, apiKey string) (*cardanoTxResp, error) {
	resp, err := blockfrostGet(ctx, "txs/"+txHash, apiKey)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil // not yet included in a block
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("blockfrost txs/%s returned status code %d", txHash, resp.StatusCode)
	}

	var tx cardanoTxResp
	if err := json.NewDecoder(resp.Body).Decode(&tx); err != nil {
		return nil, err
	}
	return &tx, nil
}

type uxtoResp []struct {
	TxHash  string `json:"tx_hash"`
	TxIndex int    `json:"tx_index"`
	Amount  []struct {
		Unit     string `json:"unit"`
		Quantity string `json:"quantity"`
	} `json:"amount"`
}

// cardanoUTXO is a parsed unspent output ready for coin selection: its "hash#index"
// reference, its lovelace value, and any native assets it carries keyed by Blockfrost
// unit (policy id + asset name hex, concatenated).
type cardanoUTXO struct {
	txIn     string
	lovelace int
	assets   map[string]int
}

// cardanoLovelaceUnit is the Blockfrost "unit" denoting ADA (everything else is a native asset).
const cardanoLovelaceUnit = "lovelace"

// parseCardanoUTXOs converts a Blockfrost address-utxos response into cardanoUTXOs, reading the
// lovelace value from the "lovelace" unit explicitly (rather than assuming Amount[0]) and
// collecting every other unit as a native asset.
func parseCardanoUTXOs(uxtos uxtoResp) ([]cardanoUTXO, error) {
	out := make([]cardanoUTXO, 0, len(uxtos))
	for _, u := range uxtos {
		utxo := cardanoUTXO{txIn: u.TxHash + "#" + strconv.Itoa(u.TxIndex)}
		for _, a := range u.Amount {
			qty, err := strconv.Atoi(a.Quantity)
			if err != nil {
				return nil, fmt.Errorf("blockfrost uxto quantity for unit %q is unparseable: %v", a.Unit, err)
			}
			if a.Unit == cardanoLovelaceUnit {
				utxo.lovelace += qty
				continue
			}
			if utxo.assets == nil {
				utxo.assets = map[string]int{}
			}
			utxo.assets[a.Unit] += qty
		}
		out = append(out, utxo)
	}
	return out, nil
}

// selectCardanoUTXOs chooses inputs to cover target lovelace. Strategy: prefer pure-ADA UTXOs
// (largest first) so the common path carries no native assets, falling back to token-bearing
// UTXOs (also largest first) only when pure-ADA funds are insufficient. It returns the selected
// inputs as "hash#index" references, their total lovelace, and the aggregated native assets
// across them (which the caller must return in the change output to preserve value). It returns
// cardanoFaucetErr when the wallet's whole balance still cannot reach target.
func selectCardanoUTXOs(utxos []cardanoUTXO, target int) (txIns []string, totalLovelace int, assets map[string]int, err error) {
	// Order pure-ADA UTXOs ahead of token-bearing ones; largest lovelace first within each group.
	order := make([]cardanoUTXO, len(utxos))
	copy(order, utxos)
	sort.SliceStable(order, func(i, j int) bool {
		iPure, jPure := len(order[i].assets) == 0, len(order[j].assets) == 0
		if iPure != jPure {
			return iPure // pure-ADA first
		}
		return order[i].lovelace > order[j].lovelace // larger first
	})

	assets = map[string]int{}
	for _, u := range order {
		txIns = append(txIns, u.txIn)
		totalLovelace += u.lovelace
		for unit, qty := range u.assets {
			assets[unit] += qty
		}
		if totalLovelace > target {
			return txIns, totalLovelace, assets, nil
		}
	}
	return nil, 0, nil, errors.New(cardanoFaucetErr)
}

// cardanoAssetID converts a Blockfrost asset unit (a 56-hex-char policy id immediately followed
// by the asset name hex) into cardano-cli's "policyId.assetNameHex" form. A unit with no asset
// name (bare policy id) yields just the policy id.
func cardanoAssetID(unit string) string {
	const policyIDLen = 56
	if len(unit) <= policyIDLen {
		return unit
	}
	return unit[:policyIDLen] + "." + unit[policyIDLen:]
}

// cardanoTxOutValue builds the value portion of a cardano-cli --tx-out for a change output
// returning changeLovelace plus every native asset in assets. Asset units are sorted so the
// output is deterministic: the two fee passes (placeholder then real) must produce a
// byte-identical tx layout for the measured size to match the final one.
func cardanoTxOutValue(changeLovelace int, assets map[string]int) string {
	value := strconv.Itoa(changeLovelace)
	units := make([]string, 0, len(assets))
	for unit := range assets {
		units = append(units, unit)
	}
	sort.Strings(units)
	for _, unit := range units {
		value += "+" + strconv.Itoa(assets[unit]) + " " + cardanoAssetID(unit)
	}
	return value
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
		if high > len(msg) {
			high = len(msg)
		}
		ss[i] = msg[i*64 : high]
	}
	return ss
}
