package util

import (
	"fmt"
	"os"
	"time"

	"github.com/carlmjohnson/versioninfo"
)

type RunFunc func([]string) error

// fatal kills the program if the provided err is not nil, logging it as well.
func Fatal(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func Version() string {
	return fmt.Sprintf(
		"Version:     %s\nCommit time: %s (%s ago)",
		versioninfo.Short(),
		versioninfo.LastCommit.Local().Format(time.DateTime),
		time.Since(versioninfo.LastCommit).Truncate(time.Second).String(),
	)
}

// Runner runs your command and does pre/post processing.
// args should not contain the name of the command/binary.
func Runner(args []string, run RunFunc) {
	if len(args) > 0 && (args[0] == "version" || args[0] == "--version") {
		fmt.Println(Version())
		return
	}
	Fatal(run(args))
}
