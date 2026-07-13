package register

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/starlinglab/integrity-v2/config"
)

// aaStoredValue mimics how AuthAttr stores the append-only "registrations" array and how
// aa.GetAttestation decodes it: the registrations are CBOR-encoded (fxamacker/cbor, which honors the
// json tags on cardanoChainData) and decoded back with DefaultMapType map[string]any. Returning the
// realistically-typed decoded value lets us exercise matchCardanoRegistration on the exact shape it
// sees in production, rather than a hand-built map that might use different types.
func aaStoredValue(t *testing.T, regs ...aaRegistration) any {
	t.Helper()
	enc, err := cbor.EncOptions{Sort: cbor.SortCanonical}.EncMode()
	if err != nil {
		t.Fatal(err)
	}
	dec, err := cbor.DecOptions{DefaultMapType: reflect.TypeOf(map[string]any{})}.DecMode()
	if err != nil {
		t.Fatal(err)
	}
	arr := make([]any, len(regs))
	for i := range regs {
		arr[i] = regs[i]
	}
	b, err := enc.Marshal(arr)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := dec.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func cardanoReg(network, txHash string) aaRegistration {
	return aaRegistration{
		Chain: "cardano",
		Data: cardanoChainData{
			CardanoChain: network,
			TxHash:       txHash,
			BlockHeight:  100,
			BlockTime:    1700000000,
			Status:       "confirmed",
		},
	}
}

func TestMatchCardanoRegistration(t *testing.T) {
	// A confirmed mainnet registration is found by mainnet and not by preview (networks are distinct).
	t.Run("matches by network", func(t *testing.T) {
		val := aaStoredValue(t, cardanoReg("mainnet", "deadbeef"))

		got, err := matchCardanoRegistration(val, "mainnet")
		if err != nil {
			t.Fatal(err)
		}
		if got == nil {
			t.Fatal("expected a match for mainnet, got nil")
		}
		if got.TxHash != "deadbeef" || got.CardanoChain != "mainnet" {
			t.Errorf("wrong match: %+v", got)
		}

		other, err := matchCardanoRegistration(val, "preview")
		if err != nil {
			t.Fatal(err)
		}
		if other != nil {
			t.Errorf("expected no preview match, got %+v", other)
		}
	})

	// The right entry is selected out of several, including a non-cardano one.
	t.Run("selects among multiple", func(t *testing.T) {
		val := aaStoredValue(t,
			aaRegistration{Chain: "numbers", Data: map[string]any{"txHash": "n1"}},
			cardanoReg("preview", "prev1"),
			cardanoReg("mainnet", "main1"),
		)
		got, err := matchCardanoRegistration(val, "mainnet")
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || got.TxHash != "main1" {
			t.Fatalf("expected mainnet tx main1, got %+v", got)
		}
	})

	// An entry with no tx hash is not a valid prior registration.
	t.Run("ignores empty tx hash", func(t *testing.T) {
		val := aaStoredValue(t, cardanoReg("mainnet", ""))
		got, err := matchCardanoRegistration(val, "mainnet")
		if err != nil {
			t.Fatal(err)
		}
		if got != nil {
			t.Errorf("expected no match for empty tx hash, got %+v", got)
		}
	})

	// A nil value (nothing stored) is "not registered", not an error.
	t.Run("nil value", func(t *testing.T) {
		got, err := matchCardanoRegistration(nil, "mainnet")
		if err != nil {
			t.Fatal(err)
		}
		if got != nil {
			t.Errorf("expected nil for nil value, got %+v", got)
		}
	})
}

func TestPendingCardanoRoundTrip(t *testing.T) {
	conf := &config.Config{}
	conf.Dirs.Cardano = t.TempDir()

	const (
		net = "mainnet"
		cid = "bafyTestCid"
	)

	// Absent record reads as (nil, nil).
	got, err := readPendingCardano(conf, net, cid)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected nil for absent record, got %+v", got)
	}

	// Write then read back.
	want := &pendingCardanoTx{Cid: cid, Network: net, TxHash: "abc123"}
	if err := writePendingCardano(conf, want); err != nil {
		t.Fatal(err)
	}
	got, err = readPendingCardano(conf, net, cid)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || *got != *want {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", got, want)
	}

	// A different network / CID does not see this record (per-network, per-CID keying).
	if other, _ := readPendingCardano(conf, "preview", cid); other != nil {
		t.Errorf("preview should not see the mainnet record, got %+v", other)
	}
	if other, _ := readPendingCardano(conf, net, "otherCid"); other != nil {
		t.Errorf("a different CID should not see this record, got %+v", other)
	}

	// Clear, then it's absent again; clearing an already-absent record is not an error.
	if err := clearPendingCardano(conf, net, cid); err != nil {
		t.Fatal(err)
	}
	if got, _ := readPendingCardano(conf, net, cid); got != nil {
		t.Fatalf("expected nil after clear, got %+v", got)
	}
	if err := clearPendingCardano(conf, net, cid); err != nil {
		t.Errorf("clearing an absent record should be a no-op, got %v", err)
	}
}

// TestPendingCardanoPathSafety verifies a hand-crafted CID with path separators cannot escape
// Dirs.Cardano.
func TestPendingCardanoPathSafety(t *testing.T) {
	conf := &config.Config{}
	conf.Dirs.Cardano = "/base/cardano"
	p := pendingCardanoPath(conf, "mainnet", "../../etc/passwd")
	if dir := filepath.Dir(p); dir != "/base/cardano" {
		t.Errorf("pending path escaped Dirs.Cardano: %s (dir %s)", p, dir)
	}
}
