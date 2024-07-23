package main

import (
	"os"

	"github.com/starlinglab/integrity-v2/relate"
	"github.com/starlinglab/integrity-v2/util"
)

func main() {
	util.Runner(os.Args[1:], relate.Run)
}
