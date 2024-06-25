package attr

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/util"
)

var (
	cid         string
	attr        string
	getAll      bool
	strInput    string
	jsonInput   string
	isEncrypted bool
	encKeyPath  string
	index       bool
)

var miniHelp = `
Supported invocations:
    attr get ...
    attr set ...

Examples:
    attr get --cid bafy... --attr file_name
    attr set --cid bafy... --attr name --str "Picture of Bob"

`

func Run(args []string) error {
	fs := flag.NewFlagSet("attr", flag.ContinueOnError)
	fs.StringVar(&cid, "cid", "", "CID of asset")
	fs.StringVar(&attr, "attr", "", "name of attribute to get/set")
	fs.BoolVar(&getAll, "all", false, "get all attributes instead of just one")
	fs.StringVar(&strInput, "str", "", "string to set as value")
	fs.StringVar(&jsonInput, "json", "", "JSON string to decode and set as value")
	fs.BoolVar(&isEncrypted, "encrypted", false, "value to get/set is encrypted")
	fs.StringVar(&encKeyPath, "key", "", "(optional) manual path to encryption key file, implies --encrypted")
	fs.BoolVar(&index, "index", false, "index value when setting")

	if len(args) == 0 {
		fs.PrintDefaults()
		fmt.Fprint(os.Stderr, miniHelp)
		return fmt.Errorf("provide subcommand like 'get' or 'set'")
	}
	cmd := args[0]
	if cmd != "set" && cmd != "get" {
		fs.PrintDefaults()
		fmt.Fprint(os.Stderr, miniHelp)
		return fmt.Errorf("supported subcommands are: get, set")
	}

	err := fs.Parse(args[1:])
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
	if cmd == "get" {
		if strInput != "" || jsonInput != "" {
			fs.PrintDefaults()
			return fmt.Errorf("\ninput flags not supported for get command")
		}
		if index {
			fs.PrintDefaults()
			return fmt.Errorf("\n--index doesn't apply to get command")
		}
	} else {
		// "set"
		if strInput != "" && jsonInput != "" {
			fs.PrintDefaults()
			return fmt.Errorf("\nonly one of --str and --json are allowed")
		}
		if strInput == "" && jsonInput == "" {
			fs.PrintDefaults()
			return fmt.Errorf("\none of --str or --json must be set")
		}
		if encKeyPath != "" {
			return fmt.Errorf("custom key file is not supported for set")
		}
		if jsonInput != "" && index {
			return fmt.Errorf("--index is only support for --str input currently")
		}
		if getAll {
			fs.PrintDefaults()
			return fmt.Errorf("\n--all doesn't apply to set command")
		}
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

	if cmd == "get" {
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
		} else {
			ae, err := aa.GetAttestation(cid, attr, aa.GetAttOpts{EncKey: encKey})
			if err == aa.ErrNeedsKey {
				return fmt.Errorf("error attestation is encrypted, use --encrypted or --key")
			}
			if err != nil {
				return fmt.Errorf("error getting attestation: %w", err)
			}
			b, err := json.MarshalIndent(ae.Attestation.Value, "", "  ")
			if err != nil {
				return fmt.Errorf("error encoding value as JSON: %w", err)
			}
			os.Stdout.Write(b)
		}

		fmt.Fprintln(os.Stderr, "\n\nNote JSON encodings are not exact canonical representations!")
		return nil
	}
	// "set"

	var val any
	var valType string
	if strInput != "" {
		val = strInput
		valType = "str"
	} else {
		err := json.Unmarshal([]byte(jsonInput), &val)
		if err != nil {
			return fmt.Errorf("error parsing --json string: %w", err)
		}
	}

	err = aa.SetAttestations(cid, index, []aa.PostKV{{Key: attr, Value: val, EncKey: encKey, Type: valType}})
	if err != nil {
		return fmt.Errorf("error setting attestation: %w", err)
	}

	return nil
}
