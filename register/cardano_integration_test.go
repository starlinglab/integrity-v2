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

// TestParseCardanoUTXOs checks that the lovelace value is read from the "lovelace" unit
// explicitly (not Amount[0]) and that native assets are collected separately (offline).
func TestParseCardanoUTXOs(t *testing.T) {
	// A UTXO whose first amount entry is a native asset, not lovelace, to prove we don't
	// blindly trust Amount[0].
	uxtos := uxtoResp{
		{
			TxHash:  "aaaa",
			TxIndex: 1,
			Amount: []struct {
				Unit     string `json:"unit"`
				Quantity string `json:"quantity"`
			}{
				{Unit: "policy1111111111111111111111111111111111111111111111111111tok", Quantity: "7"},
				{Unit: "lovelace", Quantity: "1500000"},
			},
		},
	}
	got, err := parseCardanoUTXOs(uxtos)
	if err != nil {
		t.Fatalf("parseCardanoUTXOs: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d utxos, want 1", len(got))
	}
	if got[0].txIn != "aaaa#1" {
		t.Errorf("txIn = %q, want %q", got[0].txIn, "aaaa#1")
	}
	if got[0].lovelace != 1500000 {
		t.Errorf("lovelace = %d, want 1500000 (must come from the lovelace unit, not Amount[0])", got[0].lovelace)
	}
	if qty := got[0].assets["policy1111111111111111111111111111111111111111111111111111tok"]; qty != 7 {
		t.Errorf("asset quantity = %d, want 7", qty)
	}

	// Unparseable quantity is a hard error.
	bad := uxtoResp{{TxHash: "bbbb", TxIndex: 0, Amount: []struct {
		Unit     string `json:"unit"`
		Quantity string `json:"quantity"`
	}{{Unit: "lovelace", Quantity: "not-a-number"}}}}
	if _, err := parseCardanoUTXOs(bad); err == nil {
		t.Error("expected error on unparseable quantity, got nil")
	}
}

// TestSelectCardanoUTXOs checks the prefer-pure-ADA, largest-first selection strategy and
// native-asset aggregation (offline).
func TestSelectCardanoUTXOs(t *testing.T) {
	pure := func(txIn string, lovelace int) cardanoUTXO {
		return cardanoUTXO{txIn: txIn, lovelace: lovelace}
	}
	tok := func(txIn string, lovelace int, assets map[string]int) cardanoUTXO {
		return cardanoUTXO{txIn: txIn, lovelace: lovelace, assets: assets}
	}

	// A single large pure-ADA UTXO is preferred over a larger token-bearing one.
	t.Run("prefers pure ADA", func(t *testing.T) {
		sel, total, assets, err := selectCardanoUTXOs([]cardanoUTXO{
			tok("tok#0", 9_000_000, map[string]int{"u": 5}),
			pure("pure#0", 2_000_000),
		}, cardanoFeePlaceholder)
		if err != nil {
			t.Fatal(err)
		}
		if len(sel) != 1 || sel[0] != "pure#0" {
			t.Fatalf("selected %v, want only pure#0", sel)
		}
		if total != 2_000_000 {
			t.Errorf("total = %d, want 2000000", total)
		}
		if len(assets) != 0 {
			t.Errorf("assets = %v, want empty (pure-ADA path)", assets)
		}
	})

	// Largest pure-ADA UTXO comes first.
	t.Run("largest first", func(t *testing.T) {
		sel, _, _, err := selectCardanoUTXOs([]cardanoUTXO{
			pure("small#0", 1_000_000),
			pure("big#0", 5_000_000),
		}, cardanoFeePlaceholder)
		if err != nil {
			t.Fatal(err)
		}
		if sel[0] != "big#0" {
			t.Errorf("first selected = %q, want big#0", sel[0])
		}
	})

	// No single UTXO suffices: accumulate multiple, summing lovelace.
	t.Run("multi-input accumulation", func(t *testing.T) {
		sel, total, _, err := selectCardanoUTXOs([]cardanoUTXO{
			pure("a#0", 120_000),
			pure("b#0", 120_000),
		}, cardanoFeePlaceholder)
		if err != nil {
			t.Fatal(err)
		}
		if len(sel) != 2 {
			t.Fatalf("selected %d utxos, want 2", len(sel))
		}
		if total != 240_000 {
			t.Errorf("total = %d, want 240000", total)
		}
	})

	// Token UTXOs are pulled in only when needed, and their assets aggregate.
	t.Run("token fallback aggregates assets", func(t *testing.T) {
		sel, total, assets, err := selectCardanoUTXOs([]cardanoUTXO{
			tok("t1#0", 150_000, map[string]int{"u": 3}),
			tok("t2#0", 150_000, map[string]int{"u": 4, "v": 1}),
		}, cardanoFeePlaceholder)
		if err != nil {
			t.Fatal(err)
		}
		if len(sel) != 2 {
			t.Fatalf("selected %d utxos, want 2", len(sel))
		}
		if total != 300_000 {
			t.Errorf("total = %d, want 300000", total)
		}
		if assets["u"] != 7 || assets["v"] != 1 {
			t.Errorf("assets = %v, want u=7 v=1", assets)
		}
	})

	// Whole balance below target -> faucet error.
	t.Run("insufficient funds", func(t *testing.T) {
		_, _, _, err := selectCardanoUTXOs([]cardanoUTXO{pure("a#0", 100)}, cardanoFeePlaceholder)
		if err == nil || err.Error() != cardanoFaucetErr {
			t.Errorf("err = %v, want cardanoFaucetErr", err)
		}
	})
}

// TestCardanoAssetID checks Blockfrost-unit -> cardano-cli asset-id conversion (offline).
func TestCardanoAssetID(t *testing.T) {
	policy := "0123456789012345678901234567890123456789012345678901234a" // 56 chars
	if len(policy) != 56 {
		t.Fatalf("test policy id is %d chars, want 56", len(policy))
	}
	// Policy id + asset name hex -> dotted.
	if got, want := cardanoAssetID(policy+"4d59"), policy+".4d59"; got != want {
		t.Errorf("cardanoAssetID = %q, want %q", got, want)
	}
	// Bare policy id (no asset name) -> unchanged.
	if got := cardanoAssetID(policy); got != policy {
		t.Errorf("cardanoAssetID(bare) = %q, want %q", got, policy)
	}
}

// TestCardanoTxOutValue checks the --tx-out value string, including deterministic asset
// ordering regardless of map insertion order (offline).
func TestCardanoTxOutValue(t *testing.T) {
	policyA := "a123456789012345678901234567890123456789012345678901234a"
	policyB := "b123456789012345678901234567890123456789012345678901234b"

	// Lovelace-only output is just the number.
	if got := cardanoTxOutValue(1_800_000, nil); got != "1800000" {
		t.Errorf("lovelace-only = %q, want %q", got, "1800000")
	}

	// Multi-asset output: units sorted, so the result is stable across runs/insertion order.
	want := "1800000+2 " + policyA + ".aa+5 " + policyB + ".bb"
	if got := cardanoTxOutValue(1_800_000, map[string]int{policyB + "bb": 5, policyA + "aa": 2}); got != want {
		t.Errorf("multi-asset = %q, want %q", got, want)
	}
	// Same assets, opposite insertion order -> identical string (determinism).
	if got := cardanoTxOutValue(1_800_000, map[string]int{policyA + "aa": 2, policyB + "bb": 5}); got != want {
		t.Errorf("non-deterministic output: %q", got)
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
