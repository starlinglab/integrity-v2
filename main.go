package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/starlinglab/integrity-v2/attr"
	"github.com/starlinglab/integrity-v2/dummy"
	exportproof "github.com/starlinglab/integrity-v2/export-proof"
	injectc2pa "github.com/starlinglab/integrity-v2/inject-c2pa"
	"github.com/starlinglab/integrity-v2/util"
)

// Main file for all-in-one build

var helpText = "TODO help text"

func run(cmd string, args []string) (bool, error) {
	var err error
	switch cmd {
	case "dummy":
		err = dummy.Run(args)
	case "export-proof":
		err = exportproof.Run(args)
	case "inject-c2pa":
		err = injectc2pa.Run(args)
	case "attr":
		err = attr.Run(args)
	case "-h", "--help", "help":
		fmt.Println(helpText)
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

	// Try to run command based on binary name
	// Might have been symlinked with different names
	ok, err := run(filepath.Base(os.Args[0]), os.Args[1:])
	if !ok {
		// If that failed, then use the second arg: ./integrity-v2 dummy ...
		if len(os.Args) == 1 {
			fmt.Fprintln(os.Stderr, "unknown command")
			os.Exit(1)
		}
		ok, err = run(os.Args[1], os.Args[2:])
	}
	if !ok {
		// If that fails too then give up
		fmt.Fprintln(os.Stderr, "unknown command")
		os.Exit(1)
	}

	// Command was run, either successfully or with error
	util.Fatal(err)
}
