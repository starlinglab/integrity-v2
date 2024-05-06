package main

import (
	"os"

	exportproof "github.com/starlinglab/integrity-v2/export-proof"
)

func main() {
	exportproof.Run(os.Args[1:])
}
