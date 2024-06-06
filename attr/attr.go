package attr

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/config"
)

var (
	cid         string
	attr        string
	strInput    string
	jsonInput   string
	isEncrypted bool
)

func Run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("provide subcommand like 'get' or 'set'")
	}
	cmd := args[0]
	if cmd != "set" && cmd != "get" {
		return fmt.Errorf("supported subcommands are: get, set")
	}

	fs := flag.NewFlagSet("attr", flag.ContinueOnError)
	fs.StringVar(&cid, "cid", "", "CID of asset")
	fs.StringVar(&attr, "attr", "", "name of attribute to get/set")
	fs.StringVar(&strInput, "str", "", "string to set as value")
	fs.StringVar(&jsonInput, "json", "", "JSON string to decode and set as value")
	fs.BoolVar(&isEncrypted, "encrypted", false, "value to get/set is encrypted")

	err := fs.Parse(args[1:])
	if err != nil {
		// Error is already printed
		os.Exit(1)
	}

	// Validate flags
	if cid == "" {
		return fmt.Errorf("provide CID with --cid")
	}
	if attr == "" {
		return fmt.Errorf("provide attribute name with --attr")
	}
	if cmd == "get" {
		if strInput != "" || jsonInput != "" {
			return fmt.Errorf("input flags not supported for get command")
		}
	} else {
		// "set"
		if strInput != "" && jsonInput != "" {
			return fmt.Errorf("only one of --str and --json are allowed")
		}
		if strInput == "" && jsonInput == "" {
			return fmt.Errorf("one of --str or --json must be set")
		}
		if isEncrypted {
			return fmt.Errorf("TODO support encrypted set")
		}
	}

	// Load attribute encryption key
	var encKey []byte
	if isEncrypted {
		var err error
		encKey, err = os.ReadFile(
			filepath.Join(config.GetConfig().Dirs.EncKeys, fmt.Sprintf("%s_%s.key", cid, attr)),
		)
		if err != nil {
			return fmt.Errorf("error reading key: %w", err)
		}
	}

	if cmd == "get" {
		ae, err := aa.GetAttestation(cid, attr, aa.GetAttOpts{EncKey: encKey})
		if err != nil {
			return fmt.Errorf("error getting attestation: %w", err)
		}
		b, err := json.MarshalIndent(ae.Attestation.Value, "", "  ")
		if err != nil {
			return fmt.Errorf("error encoding value as JSON: %w", err)
		}
		os.Stdout.Write(b)
		fmt.Fprintln(os.Stderr, "\n\nNote JSON encodings are not exact canonical representations!")
		return nil
	}
	// "set"

	var val any
	if strInput != "" {
		val = strInput
	} else {
		err := json.Unmarshal([]byte(jsonInput), &val)
		if err != nil {
			return fmt.Errorf("error parsing --json string: %w", err)
		}
	}

	err = aa.SetAttestations(cid, false, []aa.PostKV{{Key: attr, Value: val}})
	if err != nil {
		return fmt.Errorf("error setting attestation: %w", err)
	}

	return nil
}
