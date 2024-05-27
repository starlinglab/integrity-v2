package main

import (
	"os"

	"github.com/starlinglab/integrity-v2/attr"
	"github.com/starlinglab/integrity-v2/util"
)

func main() {
	util.Fatal(attr.Run(os.Args[1:]))
}
