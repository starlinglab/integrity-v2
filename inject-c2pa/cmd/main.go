package main

import (
	"os"

	injectc2pa "github.com/starlinglab/integrity-v2/inject-c2pa"
	"github.com/starlinglab/integrity-v2/util"
)

func main() {
	util.Runner(os.Args[1:], injectc2pa.Run)
}
