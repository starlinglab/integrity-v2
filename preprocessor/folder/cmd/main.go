package main

import (
	"os"

	folder_preprocessor "github.com/starlinglab/integrity-v2/preprocessor/folder"
)

func main() {
	folder_preprocessor.Run(os.Args[1:])
}
