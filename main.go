package main

import (
	"fmt"
	"os"

	"github.com/starlinglab/integrity-v2/c2pa"
	"github.com/starlinglab/integrity-v2/cid"
	"github.com/starlinglab/integrity-v2/decrypt"
	"github.com/starlinglab/integrity-v2/encrypt"
	"github.com/starlinglab/integrity-v2/export"
	"github.com/starlinglab/integrity-v2/genkey"
	"github.com/starlinglab/integrity-v2/get"
	preprocessorfolder "github.com/starlinglab/integrity-v2/preprocessor/folder"
	"github.com/starlinglab/integrity-v2/register"
	"github.com/starlinglab/integrity-v2/search"
	"github.com/starlinglab/integrity-v2/set"
	"github.com/starlinglab/integrity-v2/sync"
	"github.com/starlinglab/integrity-v2/upload"
	"github.com/starlinglab/integrity-v2/util"
	"github.com/starlinglab/integrity-v2/webhook"
)

// Main file for all-in-one build

var helpText = `This binary contains all the CLI tools and services in one.

Remote/network commands:
    starling attr get
    starling attr set
    starling attr export
    starling attr search

Commands to run on the server:
    starling genkey
    starling file upload
    starling file encrypt
    starling file decrypt
    starling file register
    starling file cid
    starling file c2pa

Further documentation on CLI tools is listed online:
https://github.com/starlinglab/integrity-v2/blob/main/docs/cli.md

Other than that, services are included:
	preprocessor-folder
	webhook
	sync

And finally, the version or --version command will display the build version.`

func run(cmd, subcmd string, args []string) (bool, error) {
	var allArgs []string
	if subcmd != "" {
		allArgs = append([]string{subcmd}, args...)
	}

	var err error
	switch cmd {
	case "attr":
		switch subcmd {
		case "":
			return true, fmt.Errorf("provide a subcommand")
		case "get":
			err = get.Run(args)
		case "set":
			err = set.Run(args)
		case "search":
			err = search.Run(args)
		case "export":
			err = export.Run(args)
		default:
			// Unknown command
			return false, nil
		}
	case "file":
		switch subcmd {
		case "":
			return true, fmt.Errorf("provide a subcommand")
		case "upload":
			err = upload.Run(args)
		case "encrypt":
			err = encrypt.Run(args)
		case "decrypt":
			err = decrypt.Run(args)
		case "register":
			err = register.Run(args)
		case "cid":
			err = cid.Run(args)
		case "c2pa":
			err = c2pa.Run(args)
		default:
			// Unknown command
			return false, nil
		}
	case "genkey":
		// subcmd is just another arg in this case
		err = genkey.Run(allArgs)
	case "webhook":
		err = webhook.Run(allArgs)
	case "preprocessor-folder":
		err = preprocessorfolder.Run(allArgs)
	case "sync":
		err = sync.Run(allArgs)
	case "-h", "--help", "help":
		fmt.Println(helpText)
	case "version", "--version":
		fmt.Println(util.Version())
	default:
		// Unknown command
		return false, nil
	}

	return true, err
}

func main() {
	if len(os.Args) == 1 {
		fmt.Println(helpText)
		return
	}
	var ok bool
	var err error
	if len(os.Args) == 2 {
		// Could be invalid "starling file"
		// Or valid "starling genkey"
		ok, err = run(os.Args[1], "", []string{})
	} else {
		// 2+ args after "starling", like "starling file cid"
		ok, err = run(os.Args[1], os.Args[2], os.Args[3:])
	}
	if !ok {
		// If that fails too then give up
		fmt.Fprintln(os.Stderr, "unknown command")
		os.Exit(1)
	}
	// Command was run, either successfully or with error
	util.Fatal(err)
}
