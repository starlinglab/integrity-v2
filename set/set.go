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
	attr        string
	strInput    string
	jsonInput   string
	isEncrypted bool
	index       bool
)

func Run(args []string) error {
	fs := flag.NewFlagSet("set", flag.ContinueOnError)
	fs.StringVar(&attr, "attr", "", "name of attribute to set")
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
	if attr == "" {
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

	if fs.NArg() != 1 {
		return fmt.Errorf("provide a single CID to work with")
	}
	cid := fs.Arg(0)

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
