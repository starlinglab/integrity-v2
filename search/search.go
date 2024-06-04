package search

import (
	"fmt"
	"os"

	"github.com/starlinglab/integrity-v2/aa"
)

func Run(args []string) error {
	if len(args) == 0 {
		fmt.Println(`$ search att <cid>
<list of all the attestation names>
$ search cids
<list of all the CIDs in the database>`)
		return nil
	}

	if args[0] == "att" {
		if len(args) != 2 {
			return fmt.Errorf("provide 1 CID to list attestations for")
		}
		atts, err := aa.GetAttestations(args[1])
		if err != nil {
			return fmt.Errorf("error getting attestation list: %w", err)
		}
		if len(atts) == 0 {
			fmt.Fprintln(os.Stderr, "No attestations found.")
			return nil
		}
		for k := range atts {
			fmt.Println(k)
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

	return fmt.Errorf("unknown subcommand")
}
