package search

import (
	"fmt"
	"os"
	"slices"

	"github.com/starlinglab/integrity-v2/aa"
)

const helpText = `search attr <cid>
<list of all the attribute names>

search cids
<list of all the CIDs in the database>

search index my_attr
<all the values for my_attr that are indexed>

search index my_attr my_value
<all the CIDs that have that key-value pair>`

func Run(args []string) error {
	if len(args) == 0 {
		fmt.Println(helpText)
		return nil
	}
	if args[0] == "--help" || args[0] == "help" || args[0] == "-h" {
		fmt.Println(helpText)
		return nil
	}

	if args[0] == "attr" {
		if len(args) != 2 {
			return fmt.Errorf("provide 1 CID to list attributes for")
		}
		atts, err := aa.GetAttestations(args[1])
		if err != nil {
			return fmt.Errorf("error getting attestation list: %w", err)
		}
		if len(atts) == 0 {
			fmt.Fprintln(os.Stderr, "No attributes found.")
			return nil
		}

		attNames := make([]string, len(atts))
		i := 0
		for k := range atts {
			attNames[i] = k
			i++
		}
		slices.Sort(attNames)
		for _, s := range attNames {
			fmt.Println(s)
		}

		return nil
	}
	if args[0] == "cids" {
		cids, err := aa.GetCIDs()
		if err != nil {
			return fmt.Errorf("error getting CIDs: %w", err)
		}
		for _, cid := range cids {
			fmt.Println(cid)
		}
		return nil
	}
	if args[0] == "index" {

		var list []string

		if len(args) == 2 {
			// Value search
			var err error
			list, err = aa.IndexListQuery(args[1])
			if err != nil {
				return fmt.Errorf("error querying AA index: %w", err)
			}
		} else if len(args) == 3 {
			// CID search
			var err error
			list, err = aa.IndexMatchQuery(args[1], args[2], "str")
			if err != nil {
				return fmt.Errorf("error querying AA index: %w", err)
			}
		} else {
			return fmt.Errorf("unknown index invocation")
		}

		if len(list) == 0 {
			fmt.Fprintln(os.Stderr, "No results found. Note that only string queries are currently supported.")
			return nil
		}
		for _, s := range list {
			fmt.Println(s)
		}
		return nil
	}

	return fmt.Errorf("unknown subcommand")
}
