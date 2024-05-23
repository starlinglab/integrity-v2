package main

import (
	"os"

	injectc2pa "github.com/starlinglab/integrity-v2/inject-c2pa"
)

func main() {
	injectc2pa.Run(os.Args[1:])
}
