package main

import (
	"fmt"
	"os"

	"github.com/starlinglab/integrity-v2/dummy"
)

// Main file for all-in-one build

func main() {
	if len(os.Args) == 1 {
		fmt.Println("must provide command name")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "dummy":
		dummy.Run(os.Args[2:])
	default:
		fmt.Println("unknown command name")
		os.Exit(1)
	}
}
