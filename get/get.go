package get

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"

	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/util"
)

var (
	cid         string
	attr        string
	getAll      bool
	isEncrypted bool
	encKeyPath  string
)

func Run(args []string) error {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	fs.StringVar(&cid, "cid", "", "CID of asset")
	fs.StringVar(&attr, "attr", "", "name of attribute to get")
	fs.BoolVar(&getAll, "all", false, "get all attributes instead of just one")
	fs.BoolVar(&isEncrypted, "encrypted", false, "value to get is encrypted")
	fs.StringVar(&encKeyPath, "key", "", "(optional) manual path to encryption key file, implies --encrypted")

	err := fs.Parse(args)
	if err != nil {
		// Error is already printed
		os.Exit(1)
	}

	// Validate flags
	if cid == "" {
		fs.PrintDefaults()
		return fmt.Errorf("\nprovide CID with --cid")
	}
	if attr == "" && !getAll {
		fs.PrintDefaults()
		return fmt.Errorf("\nprovide attribute name with --attr")
	}

	// Load attribute encryption key
	var encKey []byte
	if encKeyPath != "" {
		var err error
		encKey, err = os.ReadFile(encKeyPath)
		if err != nil {
			return fmt.Errorf("error reading key: %w", err)
		}
	} else if isEncrypted {
		var err error
		_, encKey, _, err = util.GenerateEncKey(cid, attr)
		if err != nil {
			return fmt.Errorf("error reading key: %w", err)
		}
	}

	if getAll {
		atts, err := aa.GetAttestations(cid)
		if err != nil {
			return fmt.Errorf("error getting attestations: %w", err)
		}
		pairs := make(map[string]any, len(atts))
		for name, att := range atts {
			if att.Attestation.Encrypted {
				pairs[name] = "*ENCRYPTED*"
			} else {
				pairs[name] = att.Attestation.Value
			}
		}
		b, err := json.MarshalIndent(pairs, "", "  ")
		if err != nil {
			return fmt.Errorf("error encoding value as JSON: %w", err)
		}
		os.Stdout.Write(b)
		fmt.Fprintln(os.Stderr, "\n\nThis is not an exact canonical representation.")
	} else {
		ae, err := aa.GetAttestation(cid, attr, aa.GetAttOpts{EncKey: encKey})
		if err == aa.ErrNeedsKey {
			return fmt.Errorf("error attestation is encrypted, use --encrypted or --key")
		}
		if err != nil {
			return fmt.Errorf("error getting attestation: %w", err)
		}

		kind := reflect.TypeOf(ae.Attestation.Value).Kind()
		if kind == reflect.Slice || kind == reflect.Struct || kind == reflect.Map ||
			kind == reflect.Array {
			// Not a simple single value
			b, err := json.MarshalIndent(ae.Attestation.Value, "", "  ")
			if err != nil {
				return fmt.Errorf("error encoding value as JSON: %w", err)
			}
			os.Stdout.Write(b)
			fmt.Fprintln(os.Stderr, "\n\nThis is not an exact canonical representation.")
		} else {
			fmt.Println(ae.Attestation.Value)
		}
	}

	return nil
}
