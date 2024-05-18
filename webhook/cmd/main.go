package main

import (
	"os"

	webhook "github.com/starlinglab/integrity-v2/webhook"
)

func main() {
	webhook.Run(os.Args[1:])
}
