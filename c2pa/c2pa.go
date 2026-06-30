package c2pa

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
	manifestName string
	dryRun       bool
	signer       string
	cid          string
)

func Run(args []string) error {
	fs := flag.NewFlagSet("c2pa", flag.ContinueOnError)
	fs.StringVar(&manifestName, "manifest", "", "name of the C2PA manifest template")
	fs.BoolVar(&dryRun, "dry-run", false, "show manifest without injecting any files")
	fs.StringVar(&signer, "signer", "local", "signer backend: local or trufo")

	if err := fs.Parse(args); err != nil {
		// Error is already printed
		os.Exit(1)
	}

	if manifestName == "" {
		fs.PrintDefaults()
		return fmt.Errorf("\nprovide manifest name with --manifest")
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("provide a single CID to work with")
	}
	cid = fs.Arg(0)
	conf := config.GetConfig()

	var tmpOut string
	var err error
	switch signer {
	case "local":
		tmpOut, err = signLocal(conf)
	case "trufo":
		tmpOut, err = signTrufo(conf)
	default:
		return fmt.Errorf("unknown signer %q (use local or trufo)", signer)
	}
	if err != nil {
		return err
	}
	if dryRun {
		// Builders printed the preview.
		return nil
	}
	return finishExport(conf, signer, tmpOut)
}

// signLocal signs with the local c2patool binary and returns the signed temp
// file path.
func signLocal(conf *config.Config) (string, error) {
	manifestTmpl, err := buildLocalManifest(conf)
	if err != nil {
		return "", err
	}
	if dryRun {
		return "", printJSON(manifestTmpl)
	}
	manifestJson, err := json.Marshal(manifestTmpl)
	if err != nil {
		return "", fmt.Errorf("error encoding replaced manifest JSON: %w", err)
	}

	// c2patool requires a file extension, so determine it from the file and
	// give it a symlinked input. The stored output is keyed by CID with no
	// extension.
	cidPath := filepath.Join(conf.Dirs.Files, cid)
	mediaType, err := util.GuessMediaType(cidPath)
	if err != nil {
		return "", err
	}
	extension, err := extensionFor(mediaType)
	if err != nil {
		return "", err
	}

	tmpOut := filepath.Join(util.TempDir(), "inject_c2pa-")
	tmpOut += strconv.FormatUint(rand.Uint64(), 10) + "." + extension

	cidSymlink := filepath.Join(util.TempDir(), cid) + "." + extension
	os.Remove(cidSymlink) // In case it was already created
	if err := os.Symlink(cidPath, cidSymlink); err != nil {
		return "", fmt.Errorf("error creating symlink to CID file: %w", err)
	}
	defer os.Remove(cidSymlink)

	c2paPrivKey, err := os.ReadFile(conf.C2PA.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("error reading c2pa.private_key file: %w", err)
	}
	c2paSignCert, err := os.ReadFile(conf.C2PA.SignCert)
	if err != nil {
		return "", fmt.Errorf("error reading c2pa.sign_cert file: %w", err)
	}

	if conf.Bins.C2patool == "" {
		return "", fmt.Errorf("c2patool path not configured")
	}
	cmd := exec.Command(conf.Bins.C2patool, cidSymlink, "--config", string(manifestJson), "--output", tmpOut)
	// https://github.com/contentauth/c2patool/blob/main/docs/x_509.md
	cmd.Env = append(os.Environ(),
		"C2PA_PRIVATE_KEY="+string(c2paPrivKey),
		"C2PA_SIGN_CERT="+string(c2paSignCert),
	)
	toolOutput, err := cmd.CombinedOutput()
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("c2patool not found at configured path, may not be installed: %s", conf.Bins.C2patool)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%s\n", toolOutput)
		return "", fmt.Errorf("c2patool failed, see its output above. Make sure it is installed and at the configured path")
	}
	return tmpOut, nil
}

// signTrufo signs via Trufo's hosted API and returns the signed temp file path.
func signTrufo(conf *config.Config) (string, error) {
	actions, assertions, err := buildTrufoAssertions(conf)
	if err != nil {
		return "", err
	}
	if dryRun {
		return "", printJSON(map[string]any{"actions": actions, "assertions": assertions})
	}
	if conf.Trufo.ApiKey == "" {
		return "", fmt.Errorf("trufo.api_key not set in config file")
	}

	media, err := os.ReadFile(filepath.Join(conf.Dirs.Files, cid))
	if err != nil {
		return "", fmt.Errorf("error reading CID file: %w", err)
	}
	baseURL := conf.Trufo.BaseURL
	if baseURL == "" {
		baseURL = trufoDefaultBaseURL
	}
	signed, err := signWithTrufo(baseURL, conf.Trufo.ApiKey, conf.Trufo.Mode != "prod", media, actions, assertions)
	if err != nil {
		return "", err
	}

	tmpOut := filepath.Join(util.TempDir(), "inject_c2pa-"+strconv.FormatUint(rand.Uint64(), 10))
	if err := os.WriteFile(tmpOut, signed, 0o600); err != nil {
		return "", fmt.Errorf("error writing signed file: %w", err)
	}
	return tmpOut, nil
}

// finishExport calculates the signed file's CID, logs it to AA, and moves it
// into file storage. Shared by all signers.
func finishExport(conf *config.Config, signer, tmpOut string) error {
	defer os.Remove(tmpOut)

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

	c2paCidCbor, err := aa.NewCborCID(c2paCid)
	if err != nil {
		return fmt.Errorf("error parsing CID of C2PA asset (%s): %w", c2paCid, err)
	}
	err = aa.AppendAttestation(cid, "c2pa_exports", c2paExport{
		Manifest:  manifestName,
		CID:       c2paCidCbor,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Signer:    signer,
	})
	if err != nil {
		return fmt.Errorf("error logging C2PA export to AA: %w", err)
	}
	if err := aa.AddRelationship(cid, "children", "derived", c2paCid); err != nil {
		return fmt.Errorf("error setting relationship attestations: %w", err)
	}

	if err := util.MoveFile(tmpOut, c2paFinalPath); err != nil {
		return fmt.Errorf("error moving temp file into c2pa file storage: %w", err)
	}

	fmt.Printf("Injected file stored at %s\n", c2paFinalPath)
	fmt.Println("Logged C2PA export and relationship to AuthAttr to the respective attributes: c2pa_exports, children")
	return nil
}

// buildLocalManifest reads the c2patool manifest template and replaces {{vars}}
// with attributes (assertions) and VCs (credentials).
func buildLocalManifest(conf *config.Config) (map[string]any, error) {
	b, err := os.ReadFile(filepath.Join(conf.Dirs.C2PAManifestTmpls, manifestName+".json"))
	if err != nil {
		return nil, fmt.Errorf("error reading manifest: %w", err)
	}
	var manifestTmpl map[string]any
	if err := json.Unmarshal(b, &manifestTmpl); err != nil {
		return nil, fmt.Errorf("error parsing manifest: %w", err)
	}
	if _, ok := manifestTmpl["assertions"]; !ok {
		return nil, fmt.Errorf("'assertions' not in manifest template")
	}
	manifestTmpl["assertions"], err = jsonReplace(manifestTmpl["assertions"], false)
	if err != nil {
		return nil, fmt.Errorf("error replacing assertion values in manifest: %w", err)
	}
	if _, ok := manifestTmpl["credentials"]; ok {
		manifestTmpl["credentials"], err = jsonReplace(manifestTmpl["credentials"], true)
		if err != nil {
			return nil, fmt.Errorf("error replacing credential values in manifest: %w", err)
		}
	}
	return manifestTmpl, nil
}

// buildTrufoAssertions reads a Trufo template and returns its actions and
// assertions. Template shape: {"actions": [...], "assertions": [["name", {params}], ...]}.
// {{vars}} in assertions are replaced with AA attributes. The cawg_identity is
// appended from config, so templates should not include one.
func buildTrufoAssertions(conf *config.Config) ([]any, []any, error) {
	if conf.Trufo.CawgIdentityID == "" {
		return nil, nil, fmt.Errorf("trufo.cawg_identity_id not set in config file")
	}
	b, err := os.ReadFile(filepath.Join(conf.Dirs.C2PAManifestTmpls, manifestName+".json"))
	if err != nil {
		return nil, nil, fmt.Errorf("error reading manifest: %w", err)
	}
	var tmpl struct {
		Actions    []any `json:"actions"`
		Assertions []any `json:"assertions"`
	}
	if err := json.Unmarshal(b, &tmpl); err != nil {
		return nil, nil, fmt.Errorf("error parsing manifest: %w", err)
	}

	var assertions []any
	if tmpl.Assertions != nil {
		replaced, err := jsonReplace(tmpl.Assertions, false)
		if err != nil {
			return nil, nil, fmt.Errorf("error replacing assertion values: %w", err)
		}
		assertions = replaced.([]any)
	}
	assertions = append(assertions, []any{
		"cawg_identity", map[string]any{"cawg_identity_id": conf.Trufo.CawgIdentityID},
	})

	if tmpl.Actions == nil {
		tmpl.Actions = []any{}
	}
	return tmpl.Actions, assertions, nil
}

func printJSON(v any) error {
	j, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("error encoding JSON: %w", err)
	}
	os.Stdout.Write(j)
	fmt.Println()
	return nil
}

// extensionFor maps a media type to the file extension c2patool expects.
// https://github.com/contentauth/c2patool?tab=readme-ov-file#supported-file-formats
func extensionFor(mediaType string) (string, error) {
	switch mediaType {
	case "video/avi":
		return "avi", nil
	case "image/jpeg":
		return "jpeg", nil
	case "audio/mpeg":
		return "mp3", nil
	case "video/mp4":
		// Note .m4a (audio/mp4) files also end up here due to the web spec that
		// http.DetectContentType follows.
		return "mp4", nil
	case "image/png":
		return "png", nil
	case "audio/wave":
		return "wav", nil
	case "image/webp":
		return "webp", nil
	default:
		return "", fmt.Errorf("detected file type %s not supported by this application,"+
			"possibly not by c2patool either. See "+
			"https://github.com/contentauth/c2patool?tab=readme-ov-file#supported-file-formats",
			mediaType,
		)
	}
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
	Signer    string     `cbor:"signer"`
}
