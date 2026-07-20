package pfp

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/nectar"
	"github.com/starlinglab/integrity-v2/util"
)

// aaClient is the subset of *aa.AuthAttrInstance that computeAndSetPFP needs. It exists so
// tests can substitute an in-memory fake instead of going through config/network, the same way
// computeImagePFP (webhook/file.go) takes its config explicitly rather than reading globals.
type aaClient interface {
	GetAttestation(cid, attr string, opts aa.GetAttOpts) (*aa.AttEntry, error)
	SetAttestations(cid string, index bool, kvs []aa.PostKV) error
}

func Run(args []string) error {
	fs := flag.NewFlagSet("pfp", flag.ContinueOnError)
	force := fs.Bool("force", false, "recompute and overwrite an existing pfp attribute")

	err := fs.Parse(args)
	if err != nil {
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("provide a single CID to work with")
	}
	cid := fs.Arg(0)

	pfpVal, err := computeAndSetPFP(
		context.Background(), config.GetConfig(), aa.GetAAInstanceFromConfig(), cid, *force)
	if err != nil {
		return err
	}

	fmt.Println(pfpVal)
	return nil
}

// computeAndSetPFP backfills the pfp attribute for cid, returning its value (existing or freshly
// computed). Unless force is true, an existing pfp attribute short-circuits the whole operation
// (no Nectar call, no AA write) and its value is returned as-is: this command's purpose is
// filling in *missing* values, so an existing one is the expected steady state, not an error to
// route around.
func computeAndSetPFP(ctx context.Context, conf *config.Config, aaInst aaClient, cid string, force bool) (string, error) {
	if conf.Nectar.Url == "" {
		return "", fmt.Errorf("nectar is not configured, set [nectar] url in the config file")
	}

	if !force {
		ae, err := aaInst.GetAttestation(cid, "pfp", aa.GetAttOpts{})
		if err != nil && !errors.Is(err, aa.ErrNotFound) {
			return "", fmt.Errorf("checking for existing pfp attestation: %w", err)
		}
		if err == nil {
			return fmt.Sprint(ae.Attestation.Value), nil
		}
	}

	cidPath := filepath.Join(conf.Dirs.Files, cid)
	if _, err := os.Stat(cidPath); err != nil {
		return "", fmt.Errorf("file not found for CID %s: %w", cid, err)
	}

	mediaType, err := util.GuessMediaType(cidPath)
	if err != nil {
		return "", fmt.Errorf("guessing media type: %w", err)
	}
	if !nectar.SupportsMediaType(mediaType) {
		return "", fmt.Errorf("media type %s is not supported for pfp generation", mediaType)
	}

	pfpVal, err := nectar.ComputePFP(ctx, conf.Nectar.Url, conf.Nectar.Token, cidPath)
	if err != nil {
		return "", fmt.Errorf("computing pfp: %w", err)
	}

	if err := aaInst.SetAttestations(cid, true, []aa.PostKV{{Key: "pfp", Value: pfpVal, Type: "str"}}); err != nil {
		return "", fmt.Errorf("setting pfp attestation: %w", err)
	}

	return pfpVal, nil
}
