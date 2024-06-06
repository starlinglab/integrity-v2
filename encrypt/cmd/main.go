package main

import (
	"os"

	"github.com/starlinglab/integrity-v2/encrypt"
	"github.com/starlinglab/integrity-v2/util"
)

func main() {
	util.Runner(os.Args[1:], encrypt.Run)
}
