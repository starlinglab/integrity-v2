package get

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"

	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/util"
)

var (
	attr            string
	getAll          bool
	isEncrypted     bool
	encKeyPath      string
	showAttestation bool
)

func Run(args []string) error {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	fs.StringVar(&attr, "attr", "", "name of attribute to get")
	fs.BoolVar(&getAll, "all", false, "get all attributes instead of just one")
	fs.BoolVar(&isEncrypted, "encrypted", false, "value to get is encrypted")
	fs.StringVar(&encKeyPath, "key", "", "(optional) manual path to encryption key file, implies --encrypted")
	fs.BoolVar(&showAttestation, "attestation", false, "show attestation information, not just value. Note values are not decrypted for this output.")

	err := fs.Parse(args)
	if err != nil {
		// Error is already printed
		os.Exit(1)
	}

	// Validate flags
	if attr == "" && !getAll {
		fs.PrintDefaults()
		return fmt.Errorf("\nprovide attribute name with --attr")
	}
	if getAll && showAttestation {
		return fmt.Errorf("can't use --all and --attestation together")
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("provide a single CID to work with")
	}

	cid := fs.Arg(0)

	// Load attribute encryption key
	var encKey []byte
	if encKeyPath != "" {
		var err error
		encKey, err = os.ReadFile(encKeyPath)
		if err != nil {
			return fmt.Errorf("error reading key: %w", err)
		}
	} else if isEncrypted {
		if config.GetConfig().Dirs.EncKeys == "" {
			return fmt.Errorf("enc_keys path is not configured, are you on the server?")
		}
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
		leaveEnc := false
		if showAttestation {
			leaveEnc = true
		}

		ae, err := aa.GetAttestation(cid, attr, aa.GetAttOpts{EncKey: encKey, LeaveEncrypted: leaveEnc})
		if err == aa.ErrNeedsKey {
			return fmt.Errorf("error attestation is encrypted, use --encrypted or --key")
		}
		if err != nil {
			return fmt.Errorf("error getting attestation: %w", err)
		}

		if showAttestation {
			b, err := json.MarshalIndent(ae, "", "  ")
			if err != nil {
				return fmt.Errorf("error encoding value as JSON: %w", err)
			}
			os.Stdout.Write(b)
			fmt.Fprintln(os.Stderr, "\n\nThis is not an exact canonical representation.")
			return nil
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
