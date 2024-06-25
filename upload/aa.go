package upload

import (
	"fmt"
	"time"

	"github.com/starlinglab/integrity-v2/aa"
)

// aaUpload is stored as an array element in AA under the uploads key
type aaUpload struct {
	ServiceName string `cbor:"service_name"`
	ServiceType string `cbor:"service_type"`
	Path        string `cbor:"path"`
	Timestamp   string `cbor:"timestamp"` // RFC 3339
}

func logUploadWithAA(cid, serviceName, serviceType, path string) error {
	err := aa.AppendAttestation(cid, "uploads", aaUpload{
		ServiceName: serviceName,
		ServiceType: serviceType,
		Path:        path,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		fmt.Println("Logged upload to AuthAttr under the attribute 'uploads'.")
	}
	return err
}
