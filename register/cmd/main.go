package main

import (
	"os"

	"github.com/starlinglab/integrity-v2/register"
	"github.com/starlinglab/integrity-v2/util"
)

func main() {
	util.Fatal(register.Run(os.Args[1:]))
}
