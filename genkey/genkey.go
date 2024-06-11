package genkey

import (
	"fmt"

	"github.com/starlinglab/integrity-v2/util"
)

func Run(args []string) error {
	if len(args) != 1 ||
		(len(args) == 1 && args[0] != "aa-enc" && args[0] != "aa-sig") {
		return fmt.Errorf(`Valid invocations:
$ genkey aa-enc
$ genkey aa-sig`)
	}
	if args[0] == "aa-sig" {
		fmt.Println(`Currently not implemented. Instead run: openssl genpkey -algorithm ED25519`)
		return nil
	}
	// "aa-enc"

	fmt.Print("CID: ")
	var cid string
	_, err := fmt.Scan(&cid)
	if err != nil {
		return err
	}

	var attr string
	fmt.Print("Attribute: ")
	_, err = fmt.Scan(&attr)
	if err != nil {
		return err
	}

	path, _, isNew, err := util.GenerateEncKey(cid, attr)
	if err != nil {
		return err
	}
	if !isNew {
		fmt.Printf("Key already exists at %s\n", path)
		return nil
	}
	fmt.Printf("Generated key was stored at %s\n", path)

	return nil
}
