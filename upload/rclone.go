package upload

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/starlinglab/integrity-v2/config"
)

type rcloneRemote struct {
	Type string
	// We don't care about any other fields
}

// rcloneHasRemote returns true if the rclone config has a remote under that name.
// It also returns the type of the remote as a string.
func rcloneHasRemote(name string) (bool, string, error) {
	cmd := exec.Command(config.GetConfig().Bins.Rclone, "config", "dump")
	b, err := cmd.Output()
	if err != nil {
		return false, "", err
	}

	var rcloneConfig map[string]*rcloneRemote
	err = json.Unmarshal(b, &rcloneConfig)
	if err != nil {
		return false, "", err
	}

	r, ok := rcloneConfig[name]
	if !ok {
		return false, "", nil
	}
	fmt.Fprintf(os.Stderr, "info: rclone remote '%s' is of type '%s'\n", name, r.Type)
	return true, r.Type, nil
}

func uploadRclone(remote, remoteType, remotePath string, cidPaths []string) error {
	for i, cidPath := range cidPaths {
		fmt.Printf("Uploading %d of %d...\n", i+1, len(cidPaths))
		cmd := exec.Command(
			config.GetConfig().Bins.Rclone,
			"copy", cidPath, remote+":"+remotePath,
			"--quiet",
		)
		rcloneOutput, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n%s\n", rcloneOutput)
			return fmt.Errorf("rclone failed, see output above if any. Error was: %w", err)
		}

		err = logUploadWithAA(filepath.Base(cidPath), remote, remoteType, remotePath)
		if err != nil {
			return fmt.Errorf("error logging upload to AuthAttr: %w", err)
		}
	}

	fmt.Println("Done.")
	return nil
}
