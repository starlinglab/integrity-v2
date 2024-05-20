package webhook

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/util"
)

func writeJsonResponse(w http.ResponseWriter, httpStatus int, data any) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		httpStatus = http.StatusInternalServerError
		jsonData = []byte(`{"error": "Internal server error: ` + err.Error() + `"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	w.Write(jsonData)
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	writeJsonResponse(w, http.StatusOK, map[string]string{"message": "pong"})
}

func handleGetCid(w http.ResponseWriter, r *http.Request) {
	cid := chi.URLParam(r, "cid")
	v, err := aa.GetAllAttributes(cid)
	if err != nil {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJsonResponse(w, http.StatusOK, CastMapForJSON(v))
}

func handleGetCidAttribute(w http.ResponseWriter, r *http.Request) {
	cid := chi.URLParam(r, "cid")
	attr := chi.URLParam(r, "attr")
	v, err := aa.GetAttribute(cid, attr)
	if err != nil {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJsonResponse(w, http.StatusOK, CastMapForJSON(v))
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	// Multipart form
	err := r.ParseMultipartForm(1024 << 20) // 1024 MB
	if err != nil {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	files := r.MultipartForm.File["file"]

	if len(files) != 1 {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": "Please upload only one file as 'file'."})
		return
	}

	originalFileName := files[0].Filename
	file, err := files[0].Open()
	if err != nil {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	defer file.Close()

	var buf bytes.Buffer
	teeFile := io.TeeReader(file, &buf)
	cid := util.CalculateFileCid(teeFile)
	if cid == "" {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": "Failed to generate CID for the file."})
		return
	}
	err = CopyOutputToFilePath(&buf, originalFileName, cid)
	if err != nil {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	metadatas := r.MultipartForm.File["metadata"]
	if len(metadatas) != 1 {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": "Please upload only one metadata file as 'metadata'."})
		return
	}
	metadataFile, err := metadatas[0].Open()
	if err != nil {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	defer metadataFile.Close()
	metadataString, err := io.ReadAll(metadataFile)
	if err != nil {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	var jsonMap any
	err = json.Unmarshal(metadataString, &jsonMap)
	if err != nil {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	attributes := ParseJsonToAttributes(jsonMap)
	err = aa.PostNewAttribute(cid, attributes)
	if err != nil {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJsonResponse(w, http.StatusOK, map[string]string{"cid": cid})
}

func Run(args []string) {
	r := chi.NewRouter()
	r.Get("/ping", handlePing)
	r.Get("/c/{cid}", handleGetCid)
	r.Get("/c/{cid}/{attr}", handleGetCidAttribute)
	r.Post("/upload", handleUpload)

	host := config.GetConfig().Webhook.Host
	if host == "" {
		host = ":8080"
	}
	http.ListenAndServe(host, r)
}
