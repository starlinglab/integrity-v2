package main

import (
	"os"

	"github.com/starlinglab/integrity-v2/dummy"
)

// Main pkg and function for when tool is built standalone
// Should just need to contain this one function

func main() {
	dummy.Run(os.Args[1:])
}
