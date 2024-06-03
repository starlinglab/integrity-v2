package main

import (
	"os"

	"github.com/starlinglab/integrity-v2/getcid"
	"github.com/starlinglab/integrity-v2/util"
)

func main() {
	util.Fatal(getcid.Run(os.Args[1:]))
}
