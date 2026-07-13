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
	"strings"
	"time"

	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/util"
)

const (
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

	// Conservative byte upper bounds used by cardanoMinUTXO to size a change output. The protocol
	// min-ada floor is (cardanoUTXOEntryOverhead + sizeInBytes(TxOut)) * coinsPerUTxOByte; every
	// constant below is chosen >= the maximum real CBOR encoding of its field, so the computed size
	// (and therefore the min-ada) is always an over-estimate of the ledger's true value. That keeps
	// the change output safely above the floor (never OutputTooSmallUTxO); the small overpay returns
	// to our own wallet as change.
	//
	// cardanoPolicyIDHexLen is the length of a policy id in a Blockfrost asset unit string: 28
	// bytes encoded as hex. The remainder of the unit is the asset name hex.
	cardanoPolicyIDHexLen = 56

	cardanoUTXOEntryOverhead = 160 // ledger constant: input + utxo-map entry overhead
	cardanoTxOutArrayHdr     = 1   // CBOR header for the [address, value] array
	cardanoAddrBytes         = 59  // base addr: 57 payload bytes + 2-byte bytestring header
	cardanoCoinBytes         = 9   // max uint64 coin: 0x1b + 8 bytes
	cardanoValueArrayHdr     = 1   // header for [coin, multiasset] (multi-asset path only)
	cardanoMultiassetMapHdr  = 2   // outer policy-map header (covers up to 255 policies)
	cardanoPolicyBytes       = 30  // 28-byte policy id + 2-byte bytestring header
	cardanoPolicyMapHdr      = 2   // per-policy inner asset-map header
	cardanoAssetNameHdr      = 2   // per-asset name bytestring header (covers names up to 32 bytes)
	cardanoAssetQtyBytes     = 9   // max uint64 asset quantity
)

// errInsufficientFunds is the sentinel returned by selectCardanoUTXOs (and the in-register guards)
// when the wallet's whole balance still cannot cover the fee plus the change output's min-ada floor.
// cardanoRegister translates it into network-appropriate funding guidance (see cardanoFundingHint).
var errInsufficientFunds = errors.New("cardano wallet has insufficient funds")

// cardanoNetwork bundles everything that differs between the chains we target. It is derived once
// per registration from the --testnet flag (see cardanoNetworkFor) and threaded through tx
// construction and the Blockfrost calls so nothing is hardcoded to a single network.
type cardanoNetwork struct {
	name           string   // recorded in cardanoChainData.CardanoChain: "preview" | "mainnet"
	blockfrostBase string   // Blockfrost API base URL for this network
	cliNetworkArgs []string // cardano-cli network selector: {"--mainnet"} or {"--testnet-magic", "2"}
	keyPrefix      string   // expected Blockfrost project_id prefix; keys are network-scoped
	fundingHint    string   // guidance appended to insufficient-funds errors (faucet vs. send ADA)
}

// cardanoNetworkFor maps the --testnet flag to a concrete network. A boolean only expresses two
// networks, so we support preview (testnet) and mainnet (its absence); preprod is not reachable.
func cardanoNetworkFor(testnet bool) cardanoNetwork {
	if testnet {
		return cardanoNetwork{
			name:           "preview",
			blockfrostBase: "https://cardano-preview.blockfrost.io/api/v0/",
			cliNetworkArgs: []string{"--testnet-magic", "2"},
			keyPrefix:      "preview",
			fundingHint:    "go to the faucet: https://docs.cardano.org/cardano-testnets/tools/faucet",
		}
	}
	return cardanoNetwork{
		name:           "mainnet",
		blockfrostBase: "https://cardano-mainnet.blockfrost.io/api/v0/",
		cliNetworkArgs: []string{"--mainnet"},
		keyPrefix:      "mainnet",
		fundingHint:    "send ADA to the wallet address",
	}
}

// cardanoCheckKeyNetwork verifies a Blockfrost project_id targets the selected network. Keys are
// network-scoped (their prefix is the network name: mainnet…/preview…/preprod…), so a mismatched
// prefix means the key would talk to the wrong chain; callers reject it before doing any work.
func cardanoCheckKeyNetwork(net cardanoNetwork, key string) error {
	if !strings.HasPrefix(key, net.keyPrefix) {
		return fmt.Errorf("blockfrost key does not match the selected network (%s); expected a key beginning with %q",
			net.name, net.keyPrefix)
	}
	return nil
}

type cardanoChainData struct {
	CardanoChain string `json:"cardano_chain"`
	TxHash       string `json:"tx_hash"`
	BlockHeight  int64  `json:"block_height"`
	BlockTime    int64  `json:"block_time"` // unix seconds, from Blockfrost block_time
	Status       string `json:"status"`     // "confirmed" (only confirmed txs are persisted)
}

// storedRegistration decodes one element of the append-only "registrations" array as stored in
// AuthAttr. Each element is an aaRegistration (register.go) whose Data, for a cardano entry, is a
// cardanoChainData. AuthAttr stores CBOR and fxamacker/cbor honors these json tags, so a JSON
// round-trip of the decoded value (see existingCardanoRegistration) unmarshals cleanly.
type storedRegistration struct {
	Chain string           `json:"chain"`
	Attrs []string         `json:"attrs"`
	Data  cardanoChainData `json:"data"`
}

// existingCardanoRegistration returns a prior confirmed cardano registration of cid on the network
// named netName, or nil if there is none. It reads the append-only "registrations" attribute from
// AuthAttr and matches on chain=="cardano", the recorded network (mainnet and preview are distinct),
// and a non-empty tx hash. A missing attribute — or AA mock mode — means "not registered". This is
// the idempotency guard that lets register.Run short-circuit a re-run instead of submitting a
// duplicate transaction.
func existingCardanoRegistration(cid, netName string) (*cardanoChainData, error) {
	entry, err := aa.GetAttestation(cid, "registrations", aa.GetAttOpts{})
	if err != nil {
		if errors.Is(err, aa.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading registrations for %s: %w", cid, err)
	}
	if entry == nil {
		return nil, nil // AA mock mode, or nothing stored
	}
	return matchCardanoRegistration(entry.Attestation.Value, netName)
}

// matchCardanoRegistration finds a confirmed cardano registration for netName within the decoded
// value of the "registrations" attribute. value is the CBOR-decoded array of aaRegistration entries
// ("registrations" is append-only, so it is always an array). It is split from
// existingCardanoRegistration so the matching logic can be unit-tested without an AuthAttr server.
func matchCardanoRegistration(value any, netName string) (*cardanoChainData, error) {
	if value == nil {
		return nil, nil
	}

	// The "registrations" attribute is append-only, so its decoded value is always a JSON array.
	// Re-marshal the CBOR-decoded value and unmarshal into typed registrations (json tags drive it).
	j, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("re-encoding registrations: %w", err)
	}
	var regs []storedRegistration
	if err := json.Unmarshal(j, &regs); err != nil {
		return nil, fmt.Errorf("decoding registrations: %w", err)
	}

	for i := range regs {
		if regs[i].Chain == chainCardano && regs[i].Data.CardanoChain == netName && regs[i].Data.TxHash != "" {
			d := regs[i].Data
			return &d, nil
		}
	}
	return nil, nil
}

// cardanoTxResp is the subset of Blockfrost's GET /txs/{hash} response we need.
type cardanoTxResp struct {
	BlockHeight int64 `json:"block_height"`
	BlockTime   int64 `json:"block_time"`
}

// cardanoProtocolParams is the subset of Blockfrost's GET /epochs/latest/parameters
// response we need: the linear fee coefficients (fee = min_fee_b + min_fee_a*txSize) and the
// per-byte UTXO cost used to compute the min-ada floor on the change output.
type cardanoProtocolParams struct {
	MinFeeA int `json:"min_fee_a"`
	MinFeeB int `json:"min_fee_b"`
	// CoinsPerUTXOSize is the lovelace cost per output byte. Blockfrost sends this as a JSON
	// string (e.g. "4310"), unlike the numeric fee coefficients; getCardanoProtocolParams
	// parses it into CoinsPerUTXOByte.
	CoinsPerUTXOSize string `json:"coins_per_utxo_size"`
	CoinsPerUTXOByte int    `json:"-"` // parsed from CoinsPerUTXOSize
}

func cardanoRegister(cid, msg string, testnet bool) (*cardanoChainData, error) {
	conf := config.GetConfig()

	if conf.Bins.CardanoCli == "" {
		return nil, fmt.Errorf("no cardano-cli path set in the config")
	}
	if conf.Dirs.Cardano == "" {
		return nil, fmt.Errorf("cardano dirs are not set in config")
	}

	// --testnet selects preview; its absence selects mainnet. Everything network-specific
	// (Blockfrost endpoint, cardano-cli network arg, recorded chain name) is derived from here.
	net := cardanoNetworkFor(testnet)

	// Reject a Blockfrost key that does not match the selected network up front, before generating
	// keys or building a tx, so a preview key can never be used against mainnet (or vice-versa).
	if err := cardanoCheckKeyNetwork(net, conf.Cardano.BlockfrostApiKey); err != nil {
		return nil, err
	}

	// Crash-safe resume: a previous run may have submitted a tx for this CID+network but crashed
	// before the registration was logged to AuthAttr. In that case resume polling that same tx
	// instead of building and submitting a duplicate (which would pay a second fee and create a
	// second on-chain record). The pending record is cleared by register.Run only after the
	// AuthAttr append succeeds, so this path is safe to re-enter.
	pending, err := readPendingCardano(conf, net.name, cid)
	if err != nil {
		return nil, err
	}
	if pending != nil {
		fmt.Printf("Found pending cardano tx %s; resuming confirmation instead of resubmitting\n", pending.TxHash)
		tx, err := pollCardanoConfirmation(net.blockfrostBase, pending.TxHash, conf.Cardano.BlockfrostApiKey)
		if err != nil {
			return nil, fmt.Errorf("%w; if this tx was dropped by the network and will never confirm, "+
				"remove %s and re-run to submit a new transaction",
				err, pendingCardanoPath(conf, net.name, cid))
		}
		return &cardanoChainData{
			CardanoChain: net.name,
			TxHash:       pending.TxHash,
			BlockHeight:  tx.BlockHeight,
			BlockTime:    tx.BlockTime,
			Status:       "confirmed",
		}, nil
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
		err = runCardanoCmd(append([]string{
			"address",
			"build",
			"--payment-verification-key-file",
			filepath.Join(conf.Dirs.Cardano, "payment.vkey"),
			"--out-file",
			filepath.Join(conf.Dirs.Cardano, "paymentNoStake.addr"),
		}, net.cliNetworkArgs...)...)
		if err != nil {
			return nil, err
		}
	}

	// Get UTXOs
	fmt.Println("Getting UXTOs")
	addrBytes, err := os.ReadFile(filepath.Join(conf.Dirs.Cardano, "paymentNoStake.addr"))
	if err != nil {
		return nil, err
	}
	// cardano-cli writes the bare address, but trim any stray whitespace/newline so it can never
	// corrupt the request URL (which would itself trigger a Blockfrost 400).
	addr := strings.TrimSpace(string(addrBytes))
	req, err := http.NewRequest("GET", net.blockfrostBase+"addresses/"+addr+"/utxos", nil)
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
		return nil, fmt.Errorf("address (%s) has no funds, %s", addr, net.fundingHint)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	// Any non-200 (e.g. 403 invalid token, 402 usage limit, 429 rate limit) returns a Blockfrost
	// error object, not the UTXO array; surface its body instead of an opaque unmarshal error.
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("blockfrost addresses/%s/utxos returned status code %d: %s",
			addr, resp.StatusCode, body)
	}
	var uxtos uxtoResp
	if err := json.Unmarshal(body, &uxtos); err != nil {
		return nil, err
	}

	// Fetch the current protocol parameters: the fee coefficients (to size the fee to the
	// actual transaction instead of overpaying a static amount) and coins_per_utxo_size (to
	// compute the min-ada floor the change output must clear). Fetched before selection because
	// the selection target depends on the min-ada value.
	pp, err := getCardanoProtocolParams(context.Background(), net.blockfrostBase, conf.Cardano.BlockfrostApiKey)
	if err != nil {
		return nil, err
	}

	// Choose UTXO(s) to spend. The single change output must cover both the fee and the protocol
	// min-ada floor, so we target minAda + cardanoFeePlaceholder (the placeholder is a
	// conservative upper bound on the real fee, computed below). Any native assets carried by the
	// selected UTXOs are returned in the change output (see buildAndSignCardanoTx) so the
	// transaction preserves value; those assets raise the min-ada floor, so we recompute minAda
	// from the actual selection and pull in more UTXOs until the change clears it. The loop is
	// bounded: each non-breaking round strictly raises the target, forcing selectCardanoUTXOs to
	// return a strictly longer prefix, and minAda is monotonic in the asset set so it cannot
	// oscillate; exhaustion surfaces as errInsufficientFunds.
	parsed, err := parseCardanoUTXOs(uxtos)
	if err != nil {
		return nil, err
	}
	var (
		txIns    []string
		quantity int
		assets   map[string]int
		minAda   int
	)
	target := cardanoFeePlaceholder + cardanoMinUTXO(pp.CoinsPerUTXOByte, nil)
	for {
		txIns, quantity, assets, err = selectCardanoUTXOs(parsed, target)
		if err != nil {
			if errors.Is(err, errInsufficientFunds) {
				return nil, fmt.Errorf("add more funds, %s", net.fundingHint)
			}
			return nil, err
		}
		minAda = cardanoMinUTXO(pp.CoinsPerUTXOByte, assets)
		if quantity >= minAda+cardanoFeePlaceholder {
			break
		}
		target = minAda + cardanoFeePlaceholder
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
	measured, err := buildAndSignCardanoTx(conf, net, addr, txIns, cardanoFeePlaceholder, quantity-cardanoFeePlaceholder, assets)
	if err != nil {
		return nil, err
	}

	// Compute the real fee from the protocol params and the measured size. The change output
	// (quantity - fee) must still clear the min-ada floor; the selection loop guarantees this
	// whenever fee <= cardanoFeePlaceholder, so this guard only fires for an unusually large fee
	// (e.g. very large metadata or many inputs).
	fee := cardanoMinFee(pp.MinFeeA, pp.MinFeeB, len(measured))
	if quantity < fee+minAda {
		return nil, fmt.Errorf("add more funds, %s", net.fundingHint)
	}

	// Pass 2: rebuild and re-sign with the computed fee and matching change output.
	txCbor, err := buildAndSignCardanoTx(conf, net, addr, txIns, fee, quantity-fee, assets)
	if err != nil {
		return nil, err
	}

	// Submit transaction to Blockfrost
	fmt.Println("Submitting transaction")
	req, err = http.NewRequest("POST", net.blockfrostBase+"tx/submit", bytes.NewReader(txCbor))
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

	// Persist the submitted tx before polling. If we crash during confirmation, a later run finds
	// this record and resumes polling the same tx instead of submitting a duplicate. register.Run
	// clears it once the registration is logged to AuthAttr.
	//
	// The tx is already submitted at this point, so a failure to write the local record must NOT
	// abort: returning here would orphan a real on-chain tx (and lose its hash), risking a duplicate
	// on retry. Surface the hash loudly and continue to poll — Run's AuthAttr append is the durable
	// record; only the crash-safe resume shortcut is lost.
	if err := writePendingCardano(conf, &pendingCardanoTx{Cid: cid, Network: net.name, TxHash: txHash}); err != nil {
		fmt.Printf("warning: could not write pending cardano record for already-submitted tx %s: %v\n", txHash, err)
	}

	// Poll until the transaction is included in a block, recording where it landed.
	// A 200 from tx/submit only means the tx was accepted into the mempool.
	fmt.Println("Waiting for on-chain confirmation")
	tx, err := pollCardanoConfirmation(net.blockfrostBase, txHash, conf.Cardano.BlockfrostApiKey)
	if err != nil {
		return nil, err
	}

	return &cardanoChainData{
		CardanoChain: net.name,
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
func buildAndSignCardanoTx(conf *config.Config, net cardanoNetwork, addr string, txIns []string, fee, change int, assets map[string]int) ([]byte, error) {
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
	signArgs := []string{
		"conway",
		"transaction",
		"sign",
		"--tx-body-file",
		filepath.Join(conf.Dirs.Cardano, "tx_new_message.draft"),
		"--signing-key-file",
		filepath.Join(conf.Dirs.Cardano, "payment.skey"),
	}
	signArgs = append(signArgs, net.cliNetworkArgs...)
	signArgs = append(signArgs, "--out-file", filepath.Join(conf.Dirs.Cardano, "tx_new_message.signed"))
	err = runCardanoCmd(signArgs...)
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
func getCardanoProtocolParams(ctx context.Context, base, apiKey string) (*cardanoProtocolParams, error) {
	resp, err := blockfrostGet(ctx, base, "epochs/latest/parameters", apiKey)
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
	// coins_per_utxo_size arrives as a string; parse it for the min-ada calculation.
	n, err := strconv.Atoi(pp.CoinsPerUTXOSize)
	if err != nil || n <= 0 {
		return nil, fmt.Errorf("blockfrost returned invalid coins_per_utxo_size %q", pp.CoinsPerUTXOSize)
	}
	pp.CoinsPerUTXOByte = n
	return &pp, nil
}

// cardanoMinUTXO returns the protocol minimum lovelace (min-ada) for a change output to a Shelley
// address carrying assets, computed as (cardanoUTXOEntryOverhead + sizeInBytes) * coinsPerUTxOByte
// where sizeInBytes is a conservative upper bound on the output's serialized size (see the byte
// constants above). Over-estimating the size means the result is always >= the ledger's true
// min-ada, so a change output funded to this value never trips OutputTooSmallUTxO. assets keys are
// Blockfrost units (56-char policy id hex + asset-name hex), matching cardanoAssetID's split.
func cardanoMinUTXO(coinsPerUTxOByte int, assets map[string]int) int {
	size := cardanoTxOutArrayHdr + cardanoAddrBytes + cardanoCoinBytes // ada-only: bare coin value
	if len(assets) > 0 {
		size += cardanoValueArrayHdr + cardanoMultiassetMapHdr
		policies := map[string]struct{}{}
		for unit := range assets {
			policy, name := unit, ""
			if len(unit) > cardanoPolicyIDHexLen { // remainder past the policy id is asset-name hex
				policy, name = unit[:cardanoPolicyIDHexLen], unit[cardanoPolicyIDHexLen:]
			}
			policies[policy] = struct{}{}
			size += cardanoAssetNameHdr + len(name)/2 + cardanoAssetQtyBytes // hex/2 = name bytes
		}
		size += len(policies) * (cardanoPolicyBytes + cardanoPolicyMapHdr)
	}
	return (cardanoUTXOEntryOverhead + size) * coinsPerUTxOByte
}

// pollCardanoConfirmation polls Blockfrost GET /txs/{hash} until txHash is included
// in a block, returning its block height and time. Blockfrost returns 404 while the
// tx is still pending. It fails if the tx is not confirmed within cardanoPollTimeout.
func pollCardanoConfirmation(base, txHash, apiKey string) (*cardanoTxResp, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cardanoPollTimeout)
	defer cancel()

	for {
		tx, err := getCardanoTx(ctx, base, txHash, apiKey)
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

// blockfrostGet issues an authenticated GET against the Blockfrost API at base+path. The caller
// owns the returned response (must close its body) and handles the status code.
func blockfrostGet(ctx context.Context, base, path, apiKey string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("project_id", apiKey)
	return http.DefaultClient.Do(req)
}

// getCardanoTx fetches a single tx from Blockfrost. It returns (nil, nil) when the tx
// is not yet on-chain (HTTP 404), so the caller can keep polling.
func getCardanoTx(ctx context.Context, base, txHash, apiKey string) (*cardanoTxResp, error) {
	resp, err := blockfrostGet(ctx, base, "txs/"+txHash, apiKey)
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
// errInsufficientFunds when the wallet's whole balance still cannot reach target.
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
	return nil, 0, nil, errInsufficientFunds
}

// cardanoAssetID converts a Blockfrost asset unit (a 56-hex-char policy id immediately followed
// by the asset name hex) into cardano-cli's "policyId.assetNameHex" form. A unit with no asset
// name (bare policy id) yields just the policy id.
func cardanoAssetID(unit string) string {
	if len(unit) <= cardanoPolicyIDHexLen {
		return unit
	}
	return unit[:cardanoPolicyIDHexLen] + "." + unit[cardanoPolicyIDHexLen:]
}

// cardanoTxOutValue builds the value portion of a cardano-cli --tx-out for a change output
// returning changeLovelace plus every native asset in assets. Asset units are sorted so the
// output is deterministic: the two fee passes (placeholder then real) must produce a
// byte-identical tx layout for the measured size to match the final one.
func cardanoTxOutValue(changeLovelace int, assets map[string]int) string {
	units := make([]string, 0, len(assets))
	for unit := range assets {
		units = append(units, unit)
	}
	sort.Strings(units)

	var value strings.Builder
	value.WriteString(strconv.Itoa(changeLovelace))
	for _, unit := range units {
		value.WriteString("+" + strconv.Itoa(assets[unit]) + " " + cardanoAssetID(unit))
	}
	return value.String()
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
		high := min(i*64+64, len(msg))
		ss[i] = msg[i*64 : high]
	}
	return ss
}
