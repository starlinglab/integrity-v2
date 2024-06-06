package main

import (
	"os"

	folder_preprocessor "github.com/starlinglab/integrity-v2/preprocessor/folder"
	"github.com/starlinglab/integrity-v2/util"
)

func main() {
	util.Runner(os.Args[1:], folder_preprocessor.Run)
}
