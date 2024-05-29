package main

import (
	"os"

	"github.com/starlinglab/integrity-v2/upload"
	"github.com/starlinglab/integrity-v2/util"
)

func main() {
	util.Fatal(upload.Run(os.Args[1:]))
}
