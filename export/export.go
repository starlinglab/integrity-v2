package export

import (
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
	format      string
	output      string
	isEncrypted bool
)

func Run(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.StringVar(&cid, "cid", "", "CID of asset")
	fs.StringVar(&attr, "attr", "", "attribute")
	fs.StringVar(&format, "format", "cbor", "proof format (cbor,vc)")
	fs.StringVar(&output, "o", "", "output path")
	fs.BoolVar(&isEncrypted, "encrypted", false, "attribute is encrypted")

	err := fs.Parse(args)
	if err != nil {
		// Error is already printed
		return fmt.Errorf("")
	}

	// Validate input
	if cid == "" {
		fs.PrintDefaults()
		return fmt.Errorf("\nprovide CID with --cid")
	}
	if attr == "" {
		fs.PrintDefaults()
		return fmt.Errorf("\nprovide attribute name with --attr")
	}
	if format != "cbor" && format != "vc" {
		fs.PrintDefaults()
		return fmt.Errorf("\nformat must be one of cbor,vc")
	}
	if output == "" {
		fs.PrintDefaults()
		return fmt.Errorf("\nmust provide output path with -o")
	}

	conf := config.GetConfig()

	// Get key
	var key []byte
	if isEncrypted {
		var err error
		key, err = os.ReadFile(filepath.Join(conf.Dirs.EncKeys, fmt.Sprintf("%s_%s.key", cid, attr)))
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
