package main

import (
	"bytes"
	"encoding/json"
	"ingest-v2/utils"
	"io"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
)

var port = os.Getenv("PORT")

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
	v, err := utils.GetAllAttributes(cid)
	if err != nil {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJsonResponse(w, http.StatusOK, utils.CastMapForJSON(v))
}

func handleGetCidAttribute(w http.ResponseWriter, r *http.Request) {
	cid := chi.URLParam(r, "cid")
	attr := chi.URLParam(r, "attr")
	v, err := utils.GetAttribute(cid, attr)
	if err != nil {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJsonResponse(w, http.StatusOK, utils.CastMapForJSON(v))
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
	cid := utils.Cid(teeFile)
	if cid == "" {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": "Failed to generate CID for the file."})
		return
	}
	err = utils.CopyOutputToFile(&buf, originalFileName, cid)
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

	var jsonMap interface{}
	err = json.Unmarshal(metadataString, &jsonMap)
	if err != nil {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	attributes := utils.ParseJsonToAttributes(jsonMap)
	err = utils.PostNewAttribute(cid, attributes)
	if err != nil {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJsonResponse(w, http.StatusOK, map[string]string{"cid": cid})
}

func main() {
	r := chi.NewRouter()
	r.Get("/ping", handlePing)
	r.Get("/c/{cid}", handleGetCid)
	r.Get("/c/{cid}/{attr}", handleGetCidAttribute)
	r.Post("/upload", handleUpload)

	if port == "" {
		port = "8080"
	}
	http.ListenAndServe(":"+port, r)
}
