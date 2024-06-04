package upload

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/util"
)

func Run(args []string) error {
	if len(args) == 1 && args[0] == "--help" {
		fmt.Println(`upload takes two or more arguments.
The first one is the storage provider and path, and the second one is the CID
to upload. You can provide multiple CIDs as well.

Some examples:

$ upload drive:dir/subdir bafy1... bafy2...
$ upload web3:some-space bafy1... bafy2...
$ upload dropbox:/ bafy1... bafy2...
$ upload drive_for_work:/ bafy1... bafy2...

upload supports any storage provider supported by rclone (https://rclone.org).
It also supports the following:
- web3.storage (web3)

web3.storage requires providing the "space" the file is uploaded to instead of
a path.

For traditional storage providers, the path is always a directory.

CIDs are retrieved from the "files" and "c2pa" storage locations.`)
		return nil
	}
	if len(args) < 2 {
		return fmt.Errorf("must provide a storage provider and CID(s), see --help")
	}

	remote, path, ok := strings.Cut(args[0], ":")
	if !ok {
		return fmt.Errorf("proper storage provider syntax is <remote>:<path>")
	}

	cidPaths, err := getCidPaths(args[1:])
	if err != nil {
		return err
	}

	if remote == "web3" {
		return uploadWeb3(path, cidPaths)
	}
	// To add another custom uploader please see "uploadRclone" in rclone.go
	// as a basic example. "logUploadWithAA" must be used!

	// All unknown remotes are assumed to be rclone remotes.

	if _, err := os.Stat(config.GetConfig().Bins.Rclone); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("rclone not found at configured path, may not be installed: %s", config.GetConfig().Bins.Rclone)
	}

	ok, remoteType, err := rcloneHasRemote(remote)
	if err != nil {
		return fmt.Errorf("error parsing rclone config: %w", err)
	}
	if !ok {
		fmt.Fprintf(
			os.Stderr,
			`remote '%s' is not yet set up in rclone. Please run "rclone config" to set it up.`,
			remote,
		)
		return fmt.Errorf("")
	}

	return uploadRclone(remote, remoteType, path, cidPaths)
}

func getCidPaths(cids []string) ([]string, error) {
	cidPaths := make([]string, len(cids))
	for i, cid := range cids {
		var err error
		cidPaths[i], err = util.CidPath(cid)
		if err != nil {
			return nil, err
		}
	}
	return cidPaths, nil
}
