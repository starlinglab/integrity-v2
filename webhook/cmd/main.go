package main

import (
	"os"

	"github.com/starlinglab/integrity-v2/util"
	webhook "github.com/starlinglab/integrity-v2/webhook"
)

func main() {
	util.Fatal(webhook.Run(os.Args[1:]))
}
