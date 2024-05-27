package main

import (
	"os"

	"github.com/starlinglab/integrity-v2/dummy"
	"github.com/starlinglab/integrity-v2/util"
)

// Main pkg and function for when tool is built standalone
// Should just need to contain this one function

func main() {
	util.Fatal(dummy.Run(os.Args[1:]))
}
