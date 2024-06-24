package webhook

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/fxamacker/cbor/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/util"
)

// Helper function to write http JSON response
func writeJsonResponse(w http.ResponseWriter, httpStatus int, data any) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		httpStatus = http.StatusInternalServerError
		jsonData = []byte(`{"error": "Internal server error: ` + err.Error() + `"}`)
	}
	w.WriteHeader(httpStatus)
	_, err = w.Write(jsonData)
	if err != nil {
		log.Println("failed to write response:", err)
	}
}

// Handle ping request
func handlePing(w http.ResponseWriter, r *http.Request) {
	writeJsonResponse(w, http.StatusOK, map[string]string{"message": "pong"})
}

// Handle quest to get all attributes for a CID
func handleGetCid(w http.ResponseWriter, r *http.Request) { //nolint:unused
	cid := chi.URLParam(r, "cid")
	v, err := aa.GetAttestations(cid)
	if err != nil {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJsonResponse(w, http.StatusOK, v)
}

// Handle request to get a specific attribute for a CID
func handleGetCidAttribute(w http.ResponseWriter, r *http.Request) { //nolint:unused
	cid := chi.URLParam(r, "cid")
	attr := chi.URLParam(r, "attr")
	v, err := aa.GetAttestation(cid, attr, aa.GetAttOpts{
		EncKey:         nil,
		LeaveEncrypted: false,
		Format:         "",
	})
	if err != nil {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJsonResponse(w, http.StatusOK, v)
}

// Handle generic file upload request, accept file and metadata from multipart form,
// calculate file CID, save to output directory, and set attestations to aa
func handleGenericFileUpload(w http.ResponseWriter, r *http.Request) {
	form, err := r.MultipartReader()
	if err != nil {
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	outputDirectory, err := getFileOutputDirectory()
	if err != nil {
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	tempFile, err := os.CreateTemp(util.TempDir(), "integrity-v2-webhook-file_")
	if err != nil {
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())
	cid := ""
	fileAttributes := map[string]any{}
	var metadataMap map[string]any
	for {
		part, err := form.NextPart()
		if err == io.EOF {
			break
		} else if err != nil {
			writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if part.FormName() == "metadata" {
			metadataFormatType := part.Header.Get("Content-Type")
			metadataValue, err := io.ReadAll(part)
			defer part.Close()
			if err != nil {
				writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			switch metadataFormatType {
			case "application/cbor":
				err = cbor.Unmarshal(metadataValue, &metadataMap)
			case "application/json":
				err = json.Unmarshal(metadataValue, &metadataMap)
			}
			if err != nil {
				writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}

		} else if part.FormName() == "file" {
			cid, fileAttributes, err = getFileAttributesAndWriteToDest(part, tempFile)
			defer part.Close()
			if err != nil {
				writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}
	}
	if cid == "" {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": "No file provided"})
		return
	}
	err = util.MoveFile(tempFile.Name(), filepath.Join(outputDirectory, cid))
	if err != nil {
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	attributes, err := ParseMapToAttributes(cid, metadataMap, fileAttributes)
	if err != nil {
		log.Println("error parsing attributes:", err)
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	err = aa.SetAttestations(cid, true, attributes)
	if err != nil {
		log.Println("error setting attestations:", err)
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJsonResponse(w, http.StatusOK, map[string]string{"cid": cid})
}

// Run the webhook server
func Run(args []string) error {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return fmt.Errorf("JWT_SECRET not set")
	}
	jwtTokenAuth := jwtauth.New("HS256", []byte(jwtSecret), nil)

	r := chi.NewRouter()
	r.Get("/ping", handlePing)
	// r.Get("/c/{cid}", handleGetCid)
	// r.Get("/c/{cid}/{attr}", handleGetCidAttribute)
	r.Post("/browsertrix", handleBrowsertrixEvent)
	r.Route("/generic", func(r chi.Router) {
		r.Use(jwtauth.Verifier(jwtTokenAuth))
		r.Use(jwtauth.Authenticator(jwtTokenAuth))
		r.Post("/", handleGenericFileUpload)
	})

	host := config.GetConfig().Webhook.Host
	if host == "" {
		return fmt.Errorf("webhook host not set in config")
	}
	log.Println("webhook server running on", host)
	err := http.ListenAndServe(host, r)
	return err
}
