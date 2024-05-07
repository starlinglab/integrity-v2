package exportproof

import (
	"flag"
	"os"
	"path/filepath"

	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/util"
)

func Run(args []string) {
	var (
		cid     string
		attr    string
		format  string
		output  string
		keyName string
		file    string
	)

	fs := flag.NewFlagSet("export-proof", flag.ContinueOnError)
	fs.StringVar(&cid, "cid", "", "CID of asset")
	fs.StringVar(&attr, "attr", "", "attribute")
	fs.StringVar(&format, "format", "cbor", "proof format (cbor,vc)")
	fs.StringVar(&output, "o", "", "output path")
	fs.StringVar(&keyName, "key", "", "name of metadata encryption key (only needed for encrypted attributes)")
	fs.StringVar(&file, "file", "", "relative file path of asset within files dir")

	err := fs.Parse(args)
	if err != nil {
		// Error is already printed
		os.Exit(1)
	}

	// Validate input

	if (cid == "" && file == "") || (cid != "" && file != "") {
		util.Die("provide CID with --cid or file path with --file")
	}
	if attr == "" {
		util.Die("provide attribute name with --attr")
	}
	if format != "cbor" && format != "vc" {
		util.Die("format must be one of cbor,vc")
	}
	if output == "" {
		util.Die("must provide output path with -o")
	}

	conf := config.GetConfig()

	// Get key
	var key []byte
	if keyName != "" {
		var err error
		key, err = os.ReadFile(filepath.Join(conf.Dirs.MetadataEncKeys, keyName))
		if err != nil {
			util.Die("error reading key: %v", err)
		}
	}

	// Get CID if needed
	if file != "" {
		cid, err = aa.GetCIDFromPath(file)
		if err != nil {
			util.Die("error getting CID: %v", err)
		}
	}

	// Get attribute
	data, err := aa.GetAttributeRaw(
		cid, attr,
		aa.AttributeOptions{
			EncKey:         key,
			LeaveEncrypted: true,
			Format:         format,
		},
	)
	if err != nil {
		util.Die("error getting attestation: %v", err)
	}

	var f *os.File
	if output == "-" {
		f = os.Stdout
	} else {
		f, err = os.Create(output)
		if err != nil {
			util.Die("couldn't open output file: %v", err)
		}
		defer f.Close()
	}

	_, err = f.Write(data)
	if err != nil {
		util.Die("error writing output: %v", err)
	}
}
