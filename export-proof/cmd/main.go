package main

import (
	"os"

	exportproof "github.com/starlinglab/integrity-v2/export-proof"
	"github.com/starlinglab/integrity-v2/util"
)

func main() {
	util.Runner(os.Args[1:], exportproof.Run)
}
