package util

import (
	"archive/zip" // nolint:staticcheck
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"slices"
	"time"

	"path/filepath"

	"github.com/digitorus/pkcs7"
	timestamp "github.com/digitorus/timestamp"
)

type signature struct {
	R, S *big.Int
}

type waczDigestData struct {
	Path       string `json:"path"`
	Hash       string `json:"hash"`
	SignedData struct {
		Hash          string    `json:"hash"`
		Signature     string    `json:"signature"`
		PublicKey     string    `json:"publicKey,omitempty"`
		Domain        string    `json:"domain,omitempty" `
		DomainCert    string    `json:"domainCert,omitempty"`
		TimeSignature string    `json:"timeSignature,omitempty"` //
		TimestampCert string    `json:"timestampCert,omitempty"` //
		Created       time.Time `json:"created"`
		Software      string    `json:"software"`
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
	Domain            string
}

// https://github.com/webrecorder/authsign/blob/main/authsign/trusted/roots.yaml
var trustedDomainFingerprints = []string{
	// Lets Encrypt Root CA X1
	"67add1166b020ae61b8f5fc96813c04c2aa589960796865572a3c7e737613dfd",
	// Lets Encrypt Root CA X3
	"6d99fb265eb1c5b3744765fcbc648f3cd8e1bffafdc4c2f99b9d47cf7ff1c24f",
}
var trustedTimestampFingerprints = []string{
	// freetsa.org Root CA (self-signed)
	"a6379e7cecc05faa3cbf076013d745e327bbbaa38c0b9af22469d4701d18aabc",
}

func verifyWaczFileHashes(packageData *waczPackageData, fileMap map[string]*zip.File) error {
	for _, resource := range packageData.Resources {
		file, ok := fileMap[resource.Path]
		if !ok {
			return fmt.Errorf("missing file %s", resource.Path)
		}
		fileReader, err := file.Open()
		if err != nil {
			return err
		}
		defer fileReader.Close()
		h := sha256.New()
		_, err = io.Copy(h, fileReader)
		if err != nil {
			return err
		}
		if resource.Hash != "sha256:"+hex.EncodeToString(h.Sum(nil)) {
			return fmt.Errorf("hash mismatch for %s", resource.Path)
		}
	}
	return nil
}

func verifyWaczAnonymousSignature(message string, signatureBytes []byte, pubKeyBytes []byte) (bool, error) {
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

func verifyTimestamp(message []byte, signatureBytes []byte, timestampCert *x509.Certificate) (*time.Time, error) {
	tst, err := timestamp.ParseResponse(signatureBytes)
	if err != nil {
		return nil, err
	}
	if (tst.HashAlgorithm != crypto.SHA256) || (tst.HashedMessage == nil) {
		return nil, fmt.Errorf("unsupported hash algorithm or missing hashed message")
	}
	encodedMessage := base64.StdEncoding.EncodeToString(message)
	h := sha256.New()
	h.Write([]byte(encodedMessage))
	if !bytes.Equal(tst.HashedMessage, h.Sum(nil)) {
		return nil, fmt.Errorf("hash mismatch")
	}
	p7, err := pkcs7.Parse(tst.RawToken)
	if err != nil {
		return nil, err
	}
	certPool := x509.NewCertPool()
	certPool.AddCert(timestampCert)
	p7.Certificates = append(p7.Certificates, timestampCert)
	err = p7.Verify()
	if err != nil {
		return nil, err
	}
	return &tst.Time, nil
}

func verifyCertificate(certString string, trustedFingerprints []string) (*x509.Certificate, error) {
	certs := []*x509.Certificate{}
	certBytes := []byte(certString)
	for {
		block, rest := pem.Decode(certBytes)
		certBytes = rest
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse domain cert: " + err.Error())
		}
		certs = append(certs, cert)
		if block == nil || len(rest) == 0 {
			break
		}
	}
	if len(certs) < 1 {
		return nil, fmt.Errorf("no certs in domain cert")
	}

	targetCert := certs[0]
	rootCert := certs[len(certs)-1]
	h := sha256.New()
	h.Write([]byte(rootCert.Raw))
	fingerprint := h.Sum(nil)
	if !slices.Contains(trustedFingerprints, hex.EncodeToString(fingerprint[:])) {
		return nil, fmt.Errorf("untrusted domain cert")
	}
	roots := x509.NewCertPool()

	if len(certs) > 1 {
		roots.AddCert(rootCert)
	}
	intermediates := x509.NewCertPool()
	for i := 1; i < len(certs)-1; i++ {
		intermediates.AddCert(certs[i])
	}

	_, err := targetCert.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsage(x509.KeyUsageDigitalSignature), x509.ExtKeyUsageTimeStamping},
	})
	if err != nil {
		return nil, err
	}
	return targetCert, nil
}

func verifyWaczDomainSignature(
	message string,
	domain string,
	signatureBytes []byte,
	domainCertString string,
	timeSignature []byte,
	timestampCertString string,
	signatureCreated time.Time,
) (bool, error) {
	domainCert, err := verifyCertificate(domainCertString, trustedDomainFingerprints)
	if err != nil {
		return false, err
	}

	err = domainCert.VerifyHostname(domain)
	if err != nil {
		return false, err
	}

	err = domainCert.CheckSignature(x509.ECDSAWithSHA256, []byte(message), signatureBytes)
	if err != nil {
		return false, err
	}

	timestampCert, err := verifyCertificate(timestampCertString, trustedTimestampFingerprints)
	if err != nil {
		return false, err
	}

	signTime, err := verifyTimestamp(signatureBytes, timeSignature, timestampCert)
	if err != nil {
		return false, err
	}

	if timestampCert.NotBefore.Before(signatureCreated) || timestampCert.NotAfter.After(signatureCreated) {
		return false, fmt.Errorf("timestamp cert not valid at creation time")
	}

	if signatureCreated.Sub(*signTime).Abs() > 10*time.Minute {
		return false, fmt.Errorf("timestamp too far from signature creation time")

	}

	return true, nil
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
	fileMap := map[string]*zip.File{}
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
		} else {
			fileMap[file.Name] = file
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

	// verify hash of data package
	h := sha256.New()
	h.Write(dataPackageBytes)
	if digestData.Hash != "sha256:"+hex.EncodeToString(h.Sum(nil)) {
		return nil, fmt.Errorf("hash mismatch")
	}

	err = verifyWaczFileHashes(&packageData, fileMap)
	if err != nil {
		return nil, err
	}

	metadataSignature, err := base64.StdEncoding.DecodeString(digestData.SignedData.Signature)
	if err != nil {
		return nil, err
	}

	verified := false
	pubKey := []byte{}

	if digestData.SignedData.PublicKey != "" {
		pubKey, err := base64.StdEncoding.DecodeString(digestData.SignedData.PublicKey)
		if err != nil {
			return nil, err
		}

		verified, err = verifyWaczAnonymousSignature(digestData.SignedData.Hash, metadataSignature, pubKey)
		if err != nil {
			return nil, err
		}
	} else if digestData.SignedData.Domain != "" {
		if digestData.SignedData.DomainCert == "" {
			return nil, fmt.Errorf("missing domain cert")
		}
		if (digestData.SignedData.TimeSignature == "") || (digestData.SignedData.TimestampCert == "") {
			return nil, fmt.Errorf("missing time signature or timestamp cert")
		}

		timeSignature, err := base64.StdEncoding.DecodeString(digestData.SignedData.TimeSignature)
		if err != nil {
			return nil, err
		}

		verified, err = verifyWaczDomainSignature(
			digestData.SignedData.Hash,
			digestData.SignedData.Domain,
			metadataSignature,
			digestData.SignedData.DomainCert,
			timeSignature,
			digestData.SignedData.TimestampCert,
			digestData.SignedData.Created,
		)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("no public key or domain")
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
		Domain:            digestData.SignedData.Domain,
	}, nil
}
