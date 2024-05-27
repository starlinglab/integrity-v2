package exportproof

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/config"
)

var (
	cid     string
	attr    string
	format  string
	output  string
	keyName string
)

func Run(args []string) error {
	fs := flag.NewFlagSet("export-proof", flag.ContinueOnError)
	fs.StringVar(&cid, "cid", "", "CID of asset")
	fs.StringVar(&attr, "attr", "", "attribute")
	fs.StringVar(&format, "format", "cbor", "proof format (cbor,vc)")
	fs.StringVar(&output, "o", "", "output path")
	fs.StringVar(&keyName, "key", "", "name of metadata encryption key (only needed for encrypted attributes)")

	err := fs.Parse(args)
	if err != nil {
		// Error is already printed
		return fmt.Errorf("")
	}

	// Validate input
	if cid == "" {
		return fmt.Errorf("provide CID with --cid")
	}
	if attr == "" {
		return fmt.Errorf("provide attribute name with --attr")
	}
	if format != "cbor" && format != "vc" {
		return fmt.Errorf("format must be one of cbor,vc")
	}
	if output == "" {
		return fmt.Errorf("must provide output path with -o")
	}

	conf := config.GetConfig()

	// Get key
	var key []byte
	if keyName != "" {
		var err error
		key, err = os.ReadFile(filepath.Join(conf.Dirs.MetadataEncKeys, keyName))
		if err != nil {
			return fmt.Errorf("error reading key: %w", err)
		}
	}

	// Get attribute
	data, err := aa.GetAttestationRaw(
		cid, attr,
		aa.GetAttOpts{
			EncKey:         key,
			LeaveEncrypted: true,
			Format:         format,
		},
	)
	if err != nil {
		return fmt.Errorf("error getting attestation: %w", err)
	}

	var f *os.File
	if output == "-" {
		f = os.Stdout
	} else {
		f, err = os.Create(output)
		if err != nil {
			return fmt.Errorf("couldn't open output file: %w", err)
		}
		defer f.Close()
	}

	_, err = f.Write(data)
	if err != nil {
		return fmt.Errorf("error writing output: %w", err)
	}

	return nil
}
