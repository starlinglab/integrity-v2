package register

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/starlinglab/integrity-v2/config"
)

// pendingCardanoTx is the local, crash-safe record of a Cardano transaction that has been submitted
// to the network but whose registration has not yet been logged to AuthAttr. It is written to disk
// under Dirs.Cardano immediately after tx/submit succeeds and before confirmation polling, so a
// crash mid-poll lets a later run resume polling the same tx instead of building and submitting a
// duplicate (paying a second fee and creating a second on-chain record). It is cleared only after
// the AuthAttr append succeeds (see register.Run).
type pendingCardanoTx struct {
	Cid     string `json:"cid"`
	Network string `json:"network"` // cardanoNetwork.name: "mainnet" | "preview"
	TxHash  string `json:"tx_hash"`
}

// pendingCardanoPath returns the per-network, per-CID path of the pending-tx record. Keying on both
// network and CID means concurrent registrations of different assets (or the same asset on different
// networks) never share a file. netName is an internal constant ("mainnet"/"preview"); only cid comes
// from CLI input, so only it is sanitized against path separators.
func pendingCardanoPath(conf *config.Config, netName, cid string) string {
	name := fmt.Sprintf("pending-%s-%s.json", netName, sanitizePendingKey(cid))
	return filepath.Join(conf.Dirs.Cardano, name)
}

// sanitizePendingKey keeps only characters known to be filesystem-safe, replacing anything else with
// '_', so a CID can never escape Dirs.Cardano or collide with a path separator. Real CIDs (base32)
// pass through unchanged.
func sanitizePendingKey(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, s)
}

// readPendingCardano loads the pending-tx record for (netName, cid), returning (nil, nil) when no
// record exists (the common case: nothing in flight).
func readPendingCardano(conf *config.Config, netName, cid string) (*pendingCardanoTx, error) {
	b, err := os.ReadFile(pendingCardanoPath(conf, netName, cid))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading pending cardano record: %w", err)
	}
	var p pendingCardanoTx
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("parsing pending cardano record %s: %w",
			pendingCardanoPath(conf, netName, cid), err)
	}
	return &p, nil
}

// writePendingCardano persists a pending-tx record. It is called after tx/submit returns 200 and
// before confirmation polling begins.
func writePendingCardano(conf *config.Config, p *pendingCardanoTx) error {
	b, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshaling pending cardano record: %w", err)
	}
	path := pendingCardanoPath(conf, p.Network, p.Cid)
	if err := os.WriteFile(path, b, 0600); err != nil {
		return fmt.Errorf("writing pending cardano record %s: %w", path, err)
	}
	return nil
}

// clearPendingCardano removes the pending-tx record for (netName, cid). It is best-effort and
// treats an already-absent file as success, so it is safe to call unconditionally after a
// registration is logged to AuthAttr.
func clearPendingCardano(conf *config.Config, netName, cid string) error {
	err := os.Remove(pendingCardanoPath(conf, netName, cid))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing pending cardano record: %w", err)
	}
	return nil
}
