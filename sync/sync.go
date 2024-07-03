package sync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"github.com/starlinglab/integrity-v2/config"
)

func Run(args []string) error {
	if len(args) == 0 ||
		(len(args) == 1 && (args[0] == "--help" || args[0] == "help" || args[0] == "-h")) {
		fmt.Println(`The sync command uses the provided arguments to run "rclone sync" in a loop.

All arguments are passed to "rclone sync", and then the command is executed in a loop,
with a 30 second delay between runs. The loop stops if the command fails.`)
		return nil
	}

	conf := config.GetConfig()

	if conf.Bins.Rclone == "" {
		return fmt.Errorf("rclone path not configured")
	}

	for {
		cmdCtx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(cmdCtx, conf.Bins.Rclone, append([]string{"sync"}, args...)...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		// Stop command if main Go process is cancelled
		sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		go func() {
			<-sigCtx.Done()
			stop()
			if cmd.ProcessState != nil {
				return
			}
			cancel()
			os.Exit(1)
		}()

		if err := cmd.Run(); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("rclone not found at configured path, may not be installed: %s", conf.Bins.Rclone)
			}
			stop()
			return err
		}
		stop()

		time.Sleep(30 * time.Second)
	}
}
