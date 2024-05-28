package webhook

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/config"
	"github.com/starlinglab/integrity-v2/util"
)

var JWT_SECRET = os.Getenv("JWT_SECRET")

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
		fmt.Println("Failed to write response:", err)
	}
}

// middleware to validate HMAC JWT token if JWT_SECRET is set
func jwtAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if JWT_SECRET != "" {
			tokenString := r.Header.Get("Authorization")
			if tokenString == "" {
				writeJsonResponse(w, http.StatusUnauthorized, map[string]string{"error": "Missing authorization header"})
				return
			}
			tokenString = tokenString[len("Bearer "):]
			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
				}
				return []byte(JWT_SECRET), nil
			})
			if err != nil {
				writeJsonResponse(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
				return
			}

			if !token.Valid {
				writeJsonResponse(w, http.StatusUnauthorized, map[string]string{"error": "Invalid token"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Handle ping request
func handlePing(w http.ResponseWriter, r *http.Request) {
	writeJsonResponse(w, http.StatusOK, map[string]string{"message": "pong"})
}

// Handle quest to get all attributes for a CID
func handleGetCid(w http.ResponseWriter, r *http.Request) {
	cid := chi.URLParam(r, "cid")
	v, err := aa.GetAttestations(cid)
	if err != nil {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJsonResponse(w, http.StatusOK, v)
}

// Handle request to get a specific attribute for a CID
func handleGetCidAttribute(w http.ResponseWriter, r *http.Request) {
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
	metadataString := []byte{}

	outputDirectory, err := getFileOutputDirectory()
	if err != nil {
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	tempFileName := uuid.New()
	tempFilePath := filepath.Join(outputDirectory, tempFileName.String())
	fd, err := os.Create(tempFilePath)
	if err != nil {
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for {
		part, err := form.NextPart()
		if err == io.EOF {
			break
		} else if err != nil {
			writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if part.FormName() == "metadata" {
			metadataString, err = io.ReadAll(part)
			defer part.Close()
			if err != nil {
				writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			//do something with files
		} else if part.FormName() == "file" {
			_, err = io.Copy(fd, part)
			defer part.Close()
			if err != nil {
				writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}
	}
	err = fd.Close()
	if err != nil {
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	fd, err = os.Open(tempFilePath)
	if err != nil {
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	cid := util.CalculateFileCid(fd)
	if cid == "" {
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate CID for the file."})
		return
	}
	e := os.Rename(tempFilePath, filepath.Join(outputDirectory, cid))
	if e != nil {
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": e.Error()})
		return
	}

	var jsonMap map[string]any
	err = json.Unmarshal(metadataString, &jsonMap)
	if err != nil {
		writeJsonResponse(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	attributes := ParseJsonToAttributes(jsonMap)
	err = aa.SetAttestations(cid, false, attributes)
	if err != nil {
		fmt.Println("Error setting attestations:", err)
		writeJsonResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJsonResponse(w, http.StatusOK, map[string]string{"cid": cid})
}

// Run the webhook server
func Run(args []string) error {
	r := chi.NewRouter()
	r.Get("/ping", handlePing)
	r.Get("/c/{cid}", handleGetCid)
	r.Get("/c/{cid}/{attr}", handleGetCidAttribute)
	r.Route("/generic", func(r chi.Router) {
		r.Use(jwtAuthMiddleware)
		r.Post("/", handleGenericFileUpload)
	})

	host := config.GetConfig().Webhook.Host
	if host == "" {
		return fmt.Errorf("Webhook host not set in config")
	}
	fmt.Println("Webhook server running on", host)
	err := http.ListenAndServe(host, r)
	return err
}
