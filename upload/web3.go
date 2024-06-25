package upload

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"

	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/util"
)

func uploadWeb3(space string, cidPaths []string) error {
	conf := config.GetConfig()

	if _, err := os.Stat(conf.Bins.W3); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("w3 (w3cli) not found at configured path, may not be installed: %s", conf.Bins.W3)
	}

	// Set space
	cmd := exec.Command(conf.Bins.W3, "space", "use", space)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%s\n", output)
		return fmt.Errorf("w3 (w3cli) failed to use space, see output above if any. Error was: %w", err)
	}

	// https://github.com/starlinglab/integrity-v2/issues/17#issuecomment-2159049248
	fmt.Fprintln(os.Stderr,
		"warning: the whole file will be loaded into memory by w3 for upload")

	for i, cidPath := range cidPaths {
		// Use anon func to allow for safe idiomatic usage of `defer`
		err := func() error {
			fmt.Printf("Uploading %d of %d...\n", i+1, len(cidPaths))

			// First create a temporary CAR file. Using a CAR file forces web3.storage to
			// use the same CIDs as us instead of generating them in their own different
			// way (--cid-version=1 --chunker=size-1048576).

			tmpF, err := os.CreateTemp(util.TempDir(), "upload_")
			if err != nil {
				return fmt.Errorf("error creating temp CAR file: %w", err)
			}
			defer tmpF.Close()
			defer os.Remove(tmpF.Name())

			cidF, err := os.Open(cidPath)
			if err != nil {
				return fmt.Errorf("error opening CID file: %w", err)
			}
			defer cidF.Close()

			fi, err := cidF.Stat()
			if err != nil {
				return fmt.Errorf("error getting CID file info: %w", err)
			}

			// Hold file in memory for CAR creation, unless it's larger than 1 GiB
			car, err := util.GetCAR(cidF, fi.Size() > 1<<30)
			if err != nil {
				return fmt.Errorf("error calculating CAR data: %w", err)
			}
			defer util.RemoveCarTmpDatastore() //nolint:errcheck

			// Make sure CID hasn't changed
			if car.Root().String() != filepath.Base(cidPath) {
				return fmt.Errorf(
					"CAR CID doesn't match file CID: %s != %s",
					car.Root().String(), filepath.Base(cidPath),
				)
			}

			if err := car.Write(tmpF); err != nil {
				return fmt.Errorf("error writing temp CAR file: %w", err)
			}
			tmpF.Close() // Flush for w3

			// Now upload that CAR file

			cmdCtx, cancel := context.WithCancel(context.Background())
			cmd = exec.CommandContext(cmdCtx, conf.Bins.W3, "up", "--car", tmpF.Name())

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
			defer stop()

			output, err = cmd.CombinedOutput()
			if err != nil {
				fmt.Fprintf(os.Stderr, "\n%s\n", output)
				return fmt.Errorf("w3 (w3cli) failed to upload, see output above if any. Error was: %w", err)
			}

			err = logUploadWithAA(filepath.Base(cidPath), "web3", "web3.storage", space)
			if err != nil {
				return fmt.Errorf("error logging upload to AuthAttr: %w", err)
			}

			return nil
		}()
		if err != nil {
			return err
		}
	}

	fmt.Println("Done.")
	return nil
}
