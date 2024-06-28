package main

import (
	"os"

	"github.com/starlinglab/integrity-v2/c2pa"
	"github.com/starlinglab/integrity-v2/util"
)

func main() {
	util.Runner(os.Args[1:], c2pa.Run)
}
