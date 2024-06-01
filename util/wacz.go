package util

import (
	"archive/zip" // nolint:staticcheck
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"time"

	"path/filepath"
	// nolint:staticcheck
)

type signature struct {
	R, S *big.Int
}

type waczDigestData struct {
	Path       string `json:"path"`
	Hash       string `json:"hash"`
	SignedData struct {
		Hash      string    `json:"hash"`
		Signature string    `json:"signature"`
		PublicKey string    `json:"publicKey"`
		Created   time.Time `json:"created"`
		Software  string    `json:"software"`
	} `json:"signedData"`
}

type waczPackageData struct {
	Profile   string `json:"profile"`
	Resources []struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		Hash  string `json:"hash"`
		Bytes int    `json:"bytes"`
	} `json:"resources"`
	WaczVersion string    `json:"wacz_version"`
	Title       string    `json:"title"`
	Software    string    `json:"software"`
	Created     time.Time `json:"created"`
	Modified    time.Time `json:"modified"`
}

type WaczFileData struct {
	Version           string
	Created           time.Time
	Modified          time.Time
	Software          string
	Title             string
	MetadataBytes     []byte
	MetadataSignature []byte
	PubKey            []byte
}

func verifyWaczSignature(message string, signatureBytes []byte, pubKeyBytes []byte) (bool, error) {
	pub, err := x509.ParsePKIXPublicKey(pubKeyBytes)
	if err != nil {
		return false, err
	}

	publicKey, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return false, fmt.Errorf("public key is not a ECDSA key")
	}

	var s signature
	if len(signatureBytes) == 2*publicKey.Params().BitSize/8 {
		// binary concat format
		s.R = new(big.Int)
		s.R.SetBytes(signatureBytes[:len(signatureBytes)/2])
		s.S = new(big.Int)
		s.S.SetBytes(signatureBytes[len(signatureBytes)/2:])
	} else {
		// asn1 format
		rest, err := asn1.Unmarshal(signatureBytes, &s)
		if err != nil {
			return false, err
		}
		if len(rest) > 0 {
			return false, fmt.Errorf("trailing data in signature")
		}
	}

	h := sha256.New()
	h.Write([]byte(message))

	return ecdsa.Verify(publicKey, h.Sum(nil), s.R, s.S), nil
}

func CheckIsWaczFile(filePath string) bool {
	if filepath.Ext(filePath) != ".wacz" {
		return false
	}
	zipListing, err := zip.OpenReader(filePath)
	if err != nil {
		return false
	}
	found := false
	for _, file := range zipListing.File {
		if file.Name == "datapackage.json" {
			found = true
			break
		}
	}
	return found
}

func ReadAndVerifyWaczMetadata(filePath string) (*WaczFileData, error) {
	zipListing, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, err
	}
	defer zipListing.Close()
	dataPackageBytes := []byte{}
	dataPackageDigestBytes := []byte{}
	for _, file := range zipListing.File {
		if file.Name == "datapackage.json" {
			fileReader, err := file.Open()
			if err != nil {
				return nil, err
			}
			defer fileReader.Close()
			dataPackageBytes, err = io.ReadAll(fileReader)
			if err != nil {
				return nil, err
			}
		} else if file.Name == "datapackage-digest.json" {
			fileReader, err := file.Open()
			if err != nil {
				return nil, err
			}
			defer fileReader.Close()
			dataPackageDigestBytes, err = io.ReadAll(fileReader)
			if err != nil {
				return nil, err
			}
		}
	}
	if len(dataPackageBytes) == 0 || len(dataPackageDigestBytes) == 0 {
		return nil, fmt.Errorf("missing datapackage.json or datapackage-digest.json")
	}

	var digestData waczDigestData
	err = json.Unmarshal(dataPackageDigestBytes, &digestData)
	if err != nil {
		return nil, err
	}
	var packageData waczPackageData
	err = json.Unmarshal(dataPackageBytes, &packageData)
	if err != nil {
		return nil, err
	}

	metadataSignature, err := base64.StdEncoding.DecodeString(digestData.SignedData.Signature)
	if err != nil {
		return nil, err
	}

	pubKey, err := base64.StdEncoding.DecodeString(digestData.SignedData.PublicKey)
	if err != nil {
		return nil, err
	}

	verified, err := verifyWaczSignature(digestData.SignedData.Hash, metadataSignature, pubKey)
	if err != nil {
		return nil, err
	}
	if !verified {
		return nil, fmt.Errorf("signature verification failed")
	}

	return &WaczFileData{
		Version:           packageData.WaczVersion,
		Created:           packageData.Created,
		Modified:          packageData.Modified,
		Software:          packageData.Software,
		Title:             packageData.Title,
		MetadataBytes:     dataPackageBytes,
		MetadataSignature: metadataSignature,
		PubKey:            pubKey,
	}, nil
}
