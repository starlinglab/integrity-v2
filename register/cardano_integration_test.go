package register

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// knownConfirmedPreviewTx is a real, already-confirmed transaction on the Cardano
// "preview" testnet, taken from docs/cardano.md. It lets us validate the shape of
// Blockfrost's GET /txs/{hash} response without submitting anything. Override with
// CARDANO_CONFIRMED_TX if this tx is ever pruned.
const knownConfirmedPreviewTx = "83d6d34c5f75faf0d441ffad3a537e4202325bb9eec3346b402907391df70985"

func blockfrostKeyOrSkip(t *testing.T) string {
	t.Helper()
	key := os.Getenv("BLOCKFROST_PROJECT_ID")
	if key == "" {
		t.Skip("set BLOCKFROST_PROJECT_ID (a free preview project_id) to run Cardano integration tests")
	}
	return key
}

// TestBlockfrostReadPath validates our assumptions about Blockfrost's GET /txs/{hash}
// endpoint against the live preview API. It is read-only: no wallet, no funds, no submit,
// so it only needs BLOCKFROST_PROJECT_ID. It pins the two load-bearing assumptions baked
// into cardano.go's confirmation polling:
//   - an unknown / not-yet-included tx returns HTTP 404 (our "still pending" signal)
//   - a confirmed tx returns block_height and block_time as positive integers
func TestBlockfrostReadPath(t *testing.T) {
	key := blockfrostKeyOrSkip(t)
	ctx := context.Background()

	confirmed := knownConfirmedPreviewTx
	if v := os.Getenv("CARDANO_CONFIRMED_TX"); v != "" {
		confirmed = v
	}

	// 1. Confirmed tx, raw request — observe the real status code and body.
	status, body := rawBlockfrostGet(t, ctx, "txs/"+confirmed, key)
	t.Logf("confirmed tx %s: HTTP %d", confirmed, status)
	t.Logf("confirmed tx body: %s", body)
	if status != 200 {
		t.Fatalf("expected HTTP 200 for a known confirmed tx, got %d (body: %s)", status, body)
	}

	// 2. Same tx through our wrapper — assert the fields we persist actually decode.
	tx, err := getCardanoTx(ctx, confirmed, key)
	if err != nil {
		t.Fatalf("getCardanoTx on confirmed tx: %v", err)
	}
	if tx == nil {
		t.Fatal("getCardanoTx returned nil for a confirmed tx (wrongly treated it as pending)")
	}
	if tx.BlockHeight <= 0 || tx.BlockTime <= 0 {
		t.Fatalf("block_height/block_time did not decode as positive ints: %+v", tx)
	}
	t.Logf("decoded block_height=%d block_time=%d (%s)", tx.BlockHeight, tx.BlockTime,
		time.Unix(tx.BlockTime, 0).UTC())

	// 3. Bogus hash, raw — confirm "not found" really is 404, our pending signal.
	bogus := "0000000000000000000000000000000000000000000000000000000000000000"
	status, body = rawBlockfrostGet(t, ctx, "txs/"+bogus, key)
	t.Logf("unknown tx: HTTP %d body: %s", status, body)
	if status != 404 {
		t.Errorf("expected HTTP 404 for an unknown tx (our pending signal); got %d. "+
			"cardano.go treats any non-404/non-200 status as a hard error, so this "+
			"would break confirmation polling.", status)
	}

	// 4. Bogus hash through the wrapper — should be (nil, nil), i.e. "keep polling".
	tx, err = getCardanoTx(ctx, bogus, key)
	if err != nil {
		t.Fatalf("getCardanoTx on unknown tx returned an error instead of pending: %v", err)
	}
	if tx != nil {
		t.Fatalf("getCardanoTx on unknown tx returned non-nil (not treated as pending): %+v", tx)
	}
}

// TestCardanoMinFee checks the linear min-fee formula (offline, no network). For a
// script-free tx the ledger's minimum fee is min_fee_b + min_fee_a*size; cardanoMinFee
// adds the small safety margin on top.
func TestCardanoMinFee(t *testing.T) {
	// Representative mainnet params at time of writing: a=44, b=155381.
	if got, want := cardanoMinFee(44, 155381, 300), 155381+44*300+cardanoFeeMargin; got != want {
		t.Errorf("cardanoMinFee(44, 155381, 300) = %d, want %d", got, want)
	}
	// Zero size still yields the fixed term plus the margin.
	if got, want := cardanoMinFee(44, 155381, 0), 155381+cardanoFeeMargin; got != want {
		t.Errorf("cardanoMinFee(44, 155381, 0) = %d, want %d", got, want)
	}
}

// TestBlockfrostProtocolParams confirms the live preview endpoint returns the fee
// coefficients the dynamic fee calculation depends on. Read-only: needs only
// BLOCKFROST_PROJECT_ID.
func TestBlockfrostProtocolParams(t *testing.T) {
	key := blockfrostKeyOrSkip(t)
	pp, err := getCardanoProtocolParams(context.Background(), key)
	if err != nil {
		t.Fatalf("getCardanoProtocolParams: %v", err)
	}
	if pp.MinFeeA <= 0 || pp.MinFeeB <= 0 {
		t.Fatalf("expected positive fee params, got min_fee_a=%d min_fee_b=%d", pp.MinFeeA, pp.MinFeeB)
	}
	t.Logf("min_fee_a=%d min_fee_b=%d", pp.MinFeeA, pp.MinFeeB)
}

// TestCardanoRegisterE2E runs the entire chain path — build, sign, submit, and poll to
// confirmation — on the preview testnet with a synthetic message. It needs a funded
// preview wallet and cardano-cli, so it is opt-in via CARDANO_E2E=1.
//
// Required env:
//
//	BLOCKFROST_PROJECT_ID  preview project_id
//	CARDANO_CLI            path to the cardano-cli binary
//	CARDANO_DIR            dir holding (or to hold) the wallet keys + scratch files
//	CARDANO_E2E=1          explicit opt-in (this spends preview tADA and can take minutes)
//
// On the first run the wallet is generated and the test fails asking you to fund the
// printed address from the faucet; fund it, then re-run.
func TestCardanoRegisterE2E(t *testing.T) {
	if os.Getenv("CARDANO_E2E") == "" {
		t.Skip("set CARDANO_E2E=1 to run the full submit+confirm test (spends preview tADA, takes minutes)")
	}
	key := blockfrostKeyOrSkip(t)
	cli := os.Getenv("CARDANO_CLI")
	dir := os.Getenv("CARDANO_DIR")
	if cli == "" || dir == "" {
		t.Skip("set CARDANO_CLI and CARDANO_DIR to run the full e2e test")
	}

	// Point config.GetConfig() at a throwaway TOML so cardanoRegister picks up our settings
	// instead of a real deployment config.
	writeE2EConfig(t, key, cli, dir)

	msg := fmt.Sprintf(`{"synthetic":true,"note":"cardano chain e2e","run":%d}`, time.Now().Unix())

	start := time.Now()
	data, err := cardanoRegister(msg)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("cardanoRegister failed after %s: %v", elapsed, err)
	}
	t.Logf("confirmed in %s: tx=%s block_height=%d block_time=%d status=%s",
		elapsed, data.TxHash, data.BlockHeight, data.BlockTime, data.Status)

	if data.BlockHeight <= 0 || data.BlockTime <= 0 {
		t.Errorf("missing block height/time in record: %+v", data)
	}
	if data.Status != "confirmed" {
		t.Errorf("status = %q, want \"confirmed\"", data.Status)
	}
	// Sanity-check the hardcoded timeout against observed reality.
	if elapsed > cardanoPollTimeout {
		t.Errorf("confirmation took %s, exceeding cardanoPollTimeout %s — timeout may be too tight",
			elapsed, cardanoPollTimeout)
	}
}

// rawBlockfrostGet performs a bare GET against the Blockfrost preview API and returns the
// status code and body, so tests can inspect exactly what the chain returns.
func rawBlockfrostGet(t *testing.T, ctx context.Context, path, key string) (int, string) {
	t.Helper()
	resp, err := blockfrostGet(ctx, path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(b)
}

// writeE2EConfig writes a minimal throwaway config and points INTEGRITY_CONFIG_PATH at it
// (auto-restored by t.Setenv) so cardanoRegister's config.GetConfig() reads our values.
func writeE2EConfig(t *testing.T, key, cli, dir string) {
	t.Helper()
	conf := fmt.Sprintf(`[dirs]
cardano = %q

[bins]
cardano_cli = %q

[cardano]
blockfrost_api_key = %q
`, dir, cli, key)
	path := filepath.Join(t.TempDir(), "integrity-v2.toml")
	if err := os.WriteFile(path, []byte(conf), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INTEGRITY_CONFIG_PATH", path)
}
