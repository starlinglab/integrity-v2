package main

import (
	"os"

	"github.com/starlinglab/integrity-v2/genkey"
	"github.com/starlinglab/integrity-v2/util"
)

func main() {
	util.Fatal(genkey.Run(os.Args[1:]))
}
