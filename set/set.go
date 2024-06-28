package set

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
	index       bool
)

func Run(args []string) error {
	fs := flag.NewFlagSet("attr", flag.ContinueOnError)
	fs.StringVar(&cid, "cid", "", "CID of asset")
	fs.StringVar(&attr, "attr", "", "name of attribute to set")
	fs.BoolVar(&getAll, "all", false, "get all attributes instead of just one")
	fs.StringVar(&strInput, "str", "", "string to set as value")
	fs.StringVar(&jsonInput, "json", "", "JSON string to decode and set as value")
	fs.BoolVar(&isEncrypted, "encrypted", false, "value to set is encrypted")
	fs.BoolVar(&index, "index", false, "index value when setting")

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

	if strInput != "" && jsonInput != "" {
		fs.PrintDefaults()
		return fmt.Errorf("\nonly one of --str and --json are allowed")
	}
	if strInput == "" && jsonInput == "" {
		fs.PrintDefaults()
		return fmt.Errorf("\none of --str or --json must be set")
	}
	if jsonInput != "" && index {
		return fmt.Errorf("--index is only support for --str input currently")
	}
	if getAll {
		fs.PrintDefaults()
		return fmt.Errorf("\n--all doesn't apply to set command")
	}

	// Load attribute encryption key
	var encKey []byte
	if isEncrypted {
		var err error
		_, encKey, _, err = util.GenerateEncKey(cid, attr)
		if err != nil {
			return fmt.Errorf("error reading key: %w", err)
		}
	}

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
