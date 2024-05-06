package util

import (
	"fmt"
	"os"
)

func Die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}
