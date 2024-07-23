package relate

import (
	"flag"
	"fmt"
	"os"

	"github.com/starlinglab/integrity-v2/aa"
)

var (
	relType string
	parent  string
	child   string
)

func Run(args []string) error {
	fs := flag.NewFlagSet("relate", flag.ContinueOnError)
	fs.StringVar(&relType, "type", "", "relationship word/type like 'related', 'verifies', '123', etc.")
	fs.StringVar(&parent, "parent", "", "CID of parent")
	fs.StringVar(&child, "child", "", "CID of child")

	err := fs.Parse(args)
	if err != nil {
		// Error is already printed
		os.Exit(1)
	}

	if relType == "" || parent == "" || child == "" {
		fs.PrintDefaults()
		return fmt.Errorf("\nall flags must be used")
	}

	err = aa.AddRelationship(parent, "children", relType, child)
	if err != nil {
		return fmt.Errorf("error adding relationship to AuthAttr: %w", err)
	}

	fmt.Println("Added relationship to AuthAttr.")
	return nil
}
