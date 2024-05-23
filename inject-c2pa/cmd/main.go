package main

import (
	"os"

	injectc2pa "github.com/starlinglab/integrity-v2/inject-c2pa"
	"github.com/starlinglab/integrity-v2/util"
)

func main() {
	util.Fatal(injectc2pa.Run(os.Args[1:]))
}
