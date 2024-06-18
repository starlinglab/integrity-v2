package injectc2pa

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/util"
)

var (
	cid          string
	manifestName string
)

func Run(args []string) error {
	fs := flag.NewFlagSet("inject-c2pa", flag.ContinueOnError)
	fs.StringVar(&cid, "cid", "", "CID of asset")
	fs.StringVar(&manifestName, "manifest", "", "name of the C2PA manifest template")

	err := fs.Parse(args)
	if err != nil {
		// Error is already printed
		os.Exit(1)
	}

	// Validate input
	if cid == "" {
		fs.PrintDefaults()
		return fmt.Errorf("\nprovide CID with --cid")
	}
	if manifestName == "" {
		fs.PrintDefaults()
		return fmt.Errorf("\nprovide manifest name with --manifest")
	}

	conf := config.GetConfig()

	// Read manifest template and replace variables in it

	manifestPath := filepath.Join(conf.Dirs.C2PAManifestTmpls, manifestName+".json")
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("error reading manifest: %w", err)
	}
	var manifestTmpl map[string]any
	err = json.Unmarshal(b, &manifestTmpl)
	if err != nil {
		return fmt.Errorf("error parsing manifest: %w", err)
	}

	// Replace assertions with attributes
	_, ok := manifestTmpl["assertions"]
	if !ok {
		return fmt.Errorf("'assertions' not in manifest template")
	}
	manifestTmpl["assertions"], err = jsonReplace(manifestTmpl["assertions"], false)
	if err != nil {
		return fmt.Errorf("error replacing assertion values in manifest: %w", err)
	}

	// Replace credentials with VCs
	_, ok = manifestTmpl["credentials"]
	if ok {
		manifestTmpl["credentials"], err = jsonReplace(manifestTmpl["credentials"], true)
		if err != nil {
			return fmt.Errorf("error replacing credential values in manifest: %w", err)
		}
	}

	manifestJson, err := json.Marshal(manifestTmpl)
	if err != nil {
		return fmt.Errorf("error encoding replaced manifest JSON: %w", err)
	}

	// File extension is required by c2patool, so figure that out first.
	// In theory this is stored by AA, but for now let's just determine it by
	// looking at the file.
	//
	// There is an open issue for this, so eventually this code can be removed.
	// https://github.com/contentauth/c2patool/issues/150

	cidPath := filepath.Join(conf.Dirs.Files, cid)
	mediaType, err := util.GuessMediaType(cidPath)
	if err != nil {
		return err
	}

	var extension string
	// https://github.com/contentauth/c2patool?tab=readme-ov-file#supported-file-formats
	// https://cs.opensource.google/go/go/+/refs/tags/go1.22.3:src/net/http/sniff.go;l=66
	switch mediaType {
	case "video/avi":
		extension = "avi"
	case "image/jpeg":
		extension = "jpeg"
	case "audio/mpeg":
		extension = "mp3"
	case "video/mp4":
		// Note .m4a (audio/mp4) files also end up here due to the web spec that
		// http.DetectContentType follows.
		extension = "mp4"
	case "image/png":
		extension = "png"
	case "audio/wave":
		extension = "wav"
	case "image/webp":
		extension = "webp"
	default:
		return fmt.Errorf("detected file type %s not supported by this application,"+
			"possibly not by c2patool either. See "+
			"https://github.com/contentauth/c2patool?tab=readme-ov-file#supported-file-formats",
			mediaType,
		)
	}

	// Store output in temporary file (later renamed to its CID)
	tmpOut := filepath.Join(util.TempDir(), "inject_c2pa-")
	tmpOut += strconv.FormatUint(rand.Uint64(), 10) + "." + extension

	// Add extension to input file (required by c2patool, see above)
	// by creating a symbolic link in a temp dir
	cidSymlink := filepath.Join(util.TempDir(), cid) + "." + extension
	os.Remove(cidSymlink) // In case it was already created
	err = os.Symlink(cidPath, cidSymlink)
	if err != nil {
		return fmt.Errorf("error creating symlink to CID file: %w", err)
	}
	defer os.Remove(cidSymlink)

	// Load c2patool certs
	c2paPrivKey, err := os.ReadFile(conf.C2PA.PrivateKey)
	if err != nil {
		return fmt.Errorf("error reading c2pa.private_key file: %w", err)
	}
	c2paSignCert, err := os.ReadFile(conf.C2PA.SignCert)
	if err != nil {
		return fmt.Errorf("error reading c2pa.sign_cert file: %w", err)
	}

	// Run c2patool

	cmd := exec.Command(
		conf.Bins.C2patool,
		cidSymlink,
		"--config",
		string(manifestJson),
		"--output",
		tmpOut,
	)

	// Provide cert and key for c2patool
	// https://github.com/contentauth/c2patool/blob/main/docs/x_509.md
	cmd.Env = append(os.Environ(),
		"C2PA_PRIVATE_KEY="+string(c2paPrivKey),
		"C2PA_SIGN_CERT="+string(c2paSignCert),
	)

	toolOutput, err := cmd.CombinedOutput()
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("c2patool not found at configured path, may not be installed: %s", conf.Bins.C2patool)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%s\n", toolOutput)
		return fmt.Errorf("c2patool failed, see its output above. Make sure it is installed and at the configured path")
	}

	// Now that the temp output file has been created, try to remove it if any
	// errors occur later.
	defer os.Remove(tmpOut)
	// Now that symlink has been read from, it can be removed
	os.Remove(cidSymlink)

	// Calc CID and final path, move later
	f, err := os.Open(tmpOut)
	if err != nil {
		return fmt.Errorf("error opening temp file: %w", err)
	}
	defer f.Close()
	c2paCid, err := util.CalculateFileCid(f)
	if err != nil {
		return fmt.Errorf("error getting output file CID: %w", err)
	}
	c2paFinalPath := filepath.Join(conf.Dirs.Files, c2paCid)

	// Set AA data:
	// Update c2pa_exports and add relationship to exported file

	c2paCidCbor, err := aa.NewCborCID(c2paCid)
	if err != nil {
		return fmt.Errorf("error parsing CID of C2PA asset (%s): %w", c2paCid, err)
	}
	err = aa.AppendAttestation(cid, "c2pa_exports", c2paExport{
		Manifest:  manifestName,
		CID:       c2paCidCbor,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("error logging C2PA export to AA: %w", err)
	}
	err = aa.AddRelationship(cid, "children", "derived", c2paCid)
	if err != nil {
		return fmt.Errorf("error setting relationship attestations: %w", err)
	}

	// Move file if everything succeeded
	err = util.MoveFile(tmpOut, c2paFinalPath)
	if err != nil {
		return fmt.Errorf("error moving temp file into c2pa file storage: %w", err)
	}

	// Tell user
	fmt.Printf("Injected file stored at %s\n", c2paFinalPath)
	return nil
}

// jsonReplace recursively replaces {{vars}} in JSON with attributes or VCs.
// Types are changed as needed.
//
// Set vc to true to replace with VCs, false to replace with attributes.
func jsonReplace(v any, useVC bool) (any, error) {
	// Takes in a value and returns a modified one with all {{vars}} replaced
	switch vv := v.(type) {

	case string:
		if !strings.HasPrefix(vv, "{{") || !strings.HasSuffix(vv, "}}") {
			return v, nil
		}
		if useVC {
			vc, err := getVC(cid, vv[2:len(vv)-2])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", vv, err)
			}
			return vc, nil
		} else {
			ae, err := aa.GetAttestation(cid, vv[2:len(vv)-2], aa.GetAttOpts{})
			if err != nil {
				return nil, fmt.Errorf("%s: %w", vv, err)
			}
			return ae.Attestation.Value, nil
		}

	case []any:
		// Search and replace through each slice value
		for i, sv := range vv {
			var err error
			vv[i], err = jsonReplace(sv, useVC)
			if err != nil {
				return nil, err
			}
		}

	case map[string]any:
		// Search and replace through each map value
		for k, mv := range vv {
			var err error
			vv[k], err = jsonReplace(mv, useVC)
			if err != nil {
				return nil, err
			}
		}

	default:
		// Some other type that can't hold a {{var}}, like an integer
		// So ignore it
	}

	return v, nil
}

func getVC(cid, attr string) (any, error) {
	data, err := aa.GetAttestationRaw(cid, attr, aa.GetAttOpts{
		LeaveEncrypted: true,
		Format:         "vc",
	})
	if err != nil {
		return nil, err
	}
	var vc any
	err = json.Unmarshal(data, &vc)
	if err != nil {
		return nil, err
	}
	return vc, nil
}

// c2paExport is used by AA in an array under the c2pa_exports key
type c2paExport struct {
	Manifest  string     `cbor:"manifest"`
	CID       aa.CborCID `cbor:"cid"`
	Timestamp string     `cbor:"timestamp"` // RFC 3339
}
