package injectc2pa

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/util"
)

var (
	cid          string
	manifestName string
)

func Run(args []string) {
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
		util.Die("provide CID with --cid")
	}
	if manifestName == "" {
		util.Die("provide manifest name with --manifest")
	}

	conf := config.GetConfig()

	// Read manifest template and replace variables in it
	manifestPath := filepath.Join(conf.Dirs.C2PAManifestTmpls, manifestName+".json")
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		util.Die("error reading manifest: %v", err)
	}
	var manifestTmpl map[string]any
	err = json.Unmarshal(b, &manifestTmpl)
	if err != nil {
		util.Die("error parsing manifest: %v", err)
	}
	manifest, err := jsonReplace(manifestTmpl)
	if err != nil {
		util.Die("error replacing values in manifest: %v", err)
	}
	manifestJson, err := json.Marshal(manifest.(map[string]any))
	if err != nil {
		util.Die("error encoding replaced manifest JSON: %v", err)
	}

	// File extension is required by c2patool, so figure that out first.
	// In theory this is stored by AA, but for now let's just determine it by
	// looking at the file.
	//
	// There is an open issue for this, so eventually this code can be removed.
	// https://github.com/contentauth/c2patool/issues/150

	cidPath := filepath.Join(conf.Dirs.Files, cid)
	f, err := os.Open(cidPath)
	if err != nil {
		util.Die("error opening CID file: %v", err)
	}
	defer f.Close()
	header := make([]byte, 512)
	_, err = io.ReadFull(f, header)
	if err != nil {
		util.Die("error reading CID file: %v", err)
	}
	mediaType := http.DetectContentType(header)
	f.Close()

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
		util.Die("detected file type %s not supported by this application,"+
			"possibly not by c2patool either. See "+
			"https://github.com/contentauth/c2patool?tab=readme-ov-file#supported-file-formats",
			mediaType,
		)
	}

	// Store output in temporary file (later renamed to its CID)
	tmpOut := filepath.Join(os.TempDir(), "inject_c2pa-")
	tmpOut += strconv.FormatUint(rand.Uint64(), 10) + "." + extension

	// Add extension to input file (required by c2patool, see above)
	// by creating a symbolic link in a temp dir
	cidSymlink := filepath.Join(os.TempDir(), cid) + "." + extension
	os.Remove(cidSymlink) // In case it was already created
	err = os.Symlink(cidPath, cidSymlink)
	if err != nil {
		util.Die("error creating symlink to CID file: %v", err)
	}
	defer os.Remove(cidSymlink)

	// Load c2patool certs
	c2paPrivKey, err := os.ReadFile(conf.C2PA.PrivateKey)
	if err != nil {
		util.Die("error reading c2pa.private_key file: %v", err)
	}
	c2paSignCert, err := os.ReadFile(conf.C2PA.SignCert)
	if err != nil {
		util.Die("error reading c2pa.sign_cert file: %v", err)
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
		util.Die("c2patool not found at configured path, may not be installed: %s", conf.Bins.C2patool)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%s\n", toolOutput)
		util.Die("c2patool failed, see its output above. Make sure it is installed and at the configured path")
	}

	// Now that the temp output file has been created, try to remove it if any
	// errors occur later.
	defer os.Remove(tmpOut)
	// Now that symlink has been read from, it can be removed
	os.Remove(cidSymlink)

	// Calc CID and final path, move later
	f, err = os.Open(tmpOut)
	if err != nil {
		util.Die("error opening temp file: %v", err)
	}
	defer f.Close()
	c2paCid, err := util.GetCid(f)
	if err != nil {
		util.Die("error getting output file CID: %v", err)
	}
	c2paFinalPath := filepath.Join(conf.Dirs.C2PA, c2paCid)

	// Set AA data:
	// Update c2pa_exports and add relationship to exported file

	c2paExports := make([]c2paExport, 0)
	att, err := aa.GetAttestation(cid, "c2pa_exports", aa.GetAttOpts{})
	if err == nil {
		// Parse into c2paExport structs
		slice, ok := att.Attestation.Value.([]any)
		if !ok {
			util.Die("schema error: c2pa_exports is not the correct type")
		}
		for _, v := range slice {
			m, ok := v.(map[string]any)
			if !ok {
				util.Die("schema error: c2pa_exports is not the correct type")
			}
			var ce c2paExport
			err := mapstructure.Decode(m, &ce)
			if err != nil {
				util.Die("schema error: c2pa_exports is not the correct type: %v", err)
			}
			c2paExports = append(c2paExports, ce)
		}
	} else if !errors.Is(err, aa.ErrNotFound) {
		// Some unknown error
		util.Die("error getting c2pa_exports attestation: %v", err)
	}

	c2paCidCbor, err := aa.NewCborCID(c2paCid)
	if err != nil {
		util.Die("error parsing CID of C2PA asset (%s): %v", c2paCid, err)
	}

	c2paExports = append(c2paExports, c2paExport{
		Manifest:  manifestName,
		CID:       c2paCidCbor,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})

	err = aa.SetAttestations(cid, false, []aa.PostKV{{Key: "c2pa_exports", Value: c2paExports}})
	if err != nil {
		util.Die("error setting c2pa_exports attestation: %v", err)
	}
	err = aa.AddRelationship(cid, "children", "derived", c2paCid)
	if err != nil {
		util.Die("error setting relationship attestations: %v", err)
	}

	// Move file if everything succeeded
	err = util.MoveFile(tmpOut, c2paFinalPath)
	if err != nil {
		util.Die("error moving temp file into c2pa file storage: %v", err)
	}

	// Tell user
	fmt.Printf("Injected file stored at %s\n", c2paFinalPath)
}

func jsonReplace(v any) (any, error) {
	// Takes in a value and returns a modified one with all {{vars}} replaced
	switch vv := v.(type) {

	case string:
		if !strings.HasPrefix(vv, "{{") || !strings.HasSuffix(vv, "}}") {
			return v, nil
		}
		av, err := aa.GetAttestation(cid, vv[2:len(vv)-2], aa.GetAttOpts{})
		if err != nil {
			return nil, fmt.Errorf("%s: %v", vv, err)
		}
		return av.Attestation.Value, nil

	case []any:
		// Search and replace through each slice value
		for i, sv := range vv {
			var err error
			vv[i], err = jsonReplace(sv)
			if err != nil {
				return nil, err
			}
		}

	case map[string]any:
		// Search and replace through each map value
		for k, mv := range vv {
			var err error
			vv[k], err = jsonReplace(mv)
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

// c2paExport is used by AA in an array under the c2pa_exports key
type c2paExport struct {
	Manifest  string     `cbor:"manifest"`
	CID       aa.CborCID `cbor:"cid"`
	Timestamp string     `cbor:"timestamp"`
}
