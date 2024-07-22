package wacz

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
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
	"log"
	"math/big"
	"slices"
	"strings"
	"time"

	"path/filepath"

	"github.com/digitorus/pkcs7"
	timestamp "github.com/digitorus/timestamp"
	"github.com/starlinglab/integrity-v2/preprocessor/common"
)

type signature struct {
	R, S *big.Int
}

type waczDigestData struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	// SignedData could contain either PublicKey
	// or Domain, DomainCert, TimeSignature, TimestampCert, Version
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
		Version       string    `json:"version"`
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
	DigestData  *waczDigestData
	PackageData *waczPackageData
	UserAgent   string
	KeyName     string
}

// SHA-256 fingerprints of CA certs for Timestamp Authorities we trust
var trustedTimestampFingerprints = []string{
	// freetsa.org Root CA (self-signed)
	// Need to trust this because Authsign uses it
	"a6379e7cecc05faa3cbf076013d745e327bbbaa38c0b9af22469d4701d18aabc",

	// DigiCertTrustedG4RSA4096SHA256TimeStampingCA.cer
	// DigiCert's CA for timestamping operations
	// Authsign will use this in the future:
	// https://github.com/starlinglab/integrity-v2/issues/50
	"281734d4592d1291d27190709cb510b07e22c405d5e0d6119b70e73589f98acf",
}

// findUserAgent finds the user agent string in the data.warc.gz file.
func findUserAgent(packageData waczPackageData, fileMap map[string]*zip.File) (string, error) {
	var targetFile string
	for _, resource := range packageData.Resources {
		if strings.HasPrefix(resource.Path, "archive/") && (strings.HasSuffix(resource.Path, ".warc") || strings.HasSuffix(resource.Path, ".warc.gz")) {
			targetFile = resource.Path
			break
		}
	}

	if targetFile == "" || fileMap[targetFile] == nil {
		return "", fmt.Errorf("missing warc files")
	}

	file, err := fileMap[targetFile].Open()
	if err != nil {
		return "", err
	}
	defer file.Close()

	if strings.HasSuffix(targetFile, ".gz") {
		file, err = gzip.NewReader(file)
		if err != nil {
			return "", err
		}
		defer file.Close()
	}

	reader := bufio.NewReader(file)
	for {
		lineBytes, isPrefix, err := reader.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
		if isPrefix {
			// discard the rest of the long line
			for isPrefix {
				_, isPrefix, err = reader.ReadLine()
				if err != nil {
					if err == io.EOF {
						break
					}
					return "", err
				}
			}
		}
		line := string(lineBytes)
		if strings.Contains(line, "user-agent: ") || strings.Contains(line, "User-Agent: ") {
			return line[strings.Index(line, ":")+2:], nil
		}
	}
	return "", fmt.Errorf("user-agent not found")
}

// verifyFileHashes verifies the hash of files listed in the package data.
func verifyFileHashes(packageData *waczPackageData, fileMap map[string]*zip.File) error {
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

// verifyAnonymousSignature verifies a signature using an anonymous public key.
func verifyAnonymousSignature(message string, signatureBytes []byte, pubKeyBytes []byte) (bool, error) {
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

// verifyTimestamp verifies the RFC3161 timestamp token
// and signature in domain signed wacz file.
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

// verifyCertificate verifies the certificates in domain signed wacz file.
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

	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}

	intermediates := x509.NewCertPool()
	for i := 1; i < len(certs)-1; i++ {
		intermediates.AddCert(certs[i])
	}

	if trustedFingerprints != nil {
		h := sha256.New()
		h.Write([]byte(rootCert.Raw))
		fingerprint := h.Sum(nil)
		if !slices.Contains(trustedFingerprints, hex.EncodeToString(fingerprint[:])) {
			return nil, fmt.Errorf("untrusted domain cert")
		}
		// use trusted root
		roots = x509.NewCertPool()
		roots.AddCert(rootCert)
	} else {
		// use system root
		intermediates.AddCert(rootCert)
	}

	_, err = targetCert.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsage(x509.KeyUsageDigitalSignature), x509.ExtKeyUsageTimeStamping},
	})
	if err != nil {
		return nil, err
	}
	return targetCert, nil
}

// verifyDomainSignature verifies a signature in a domain signed wacz file.
func verifyDomainSignature(
	message string,
	domain string,
	signatureBytes []byte,
	domainCertString string,
	timeSignature []byte,
	timestampCertString string,
	signatureCreated time.Time,
) (bool, error) {
	// These verification steps are taken from the WACZ auth spec
	// https://specs.webrecorder.net/wacz-auth/0.1.0/#domain-name-identity-timestamp-validation

	domainCert, err := verifyCertificate(domainCertString, nil)
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

	if signatureCreated.Before(timestampCert.NotBefore) || signatureCreated.After(timestampCert.NotAfter) {
		return false, fmt.Errorf("timestamp cert not valid at creation time")
	}

	if signatureCreated.Sub(*signTime).Abs() > 10*time.Minute {
		return false, fmt.Errorf("timestamp too far from signature creation time")
	}

	return true, nil
}

// CheckIsWaczFile checks if a file is a wacz file.
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

// ReadAndVerifyWaczMetadata reads and verifies the metadata of a wacz file.
func ReadAndVerifyWaczMetadata(filePath string, anonKeys, domains []*common.AllowedKey) (*WaczFileData, error) {
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

	err = verifyFileHashes(&packageData, fileMap)
	if err != nil {
		return nil, err
	}

	metadataSignature, err := base64.StdEncoding.DecodeString(digestData.SignedData.Signature)
	if err != nil {
		return nil, err
	}

	verified := false
	var keyName string

	if digestData.SignedData.PublicKey != "" {
		// Check public key is in allow-list
		foundKey := false
		for _, k := range anonKeys {
			if k.Key == digestData.SignedData.PublicKey {
				keyName = k.Name
				foundKey = true
				break
			}
		}
		if !foundKey {
			return nil, fmt.Errorf("wacz public key was not in allow-list")
		}

		pubKey, err := base64.StdEncoding.DecodeString(digestData.SignedData.PublicKey)
		if err != nil {
			return nil, err
		}
		verified, err = verifyAnonymousSignature(digestData.SignedData.Hash, metadataSignature, pubKey)
		if err != nil {
			return nil, err
		}
	} else if digestData.SignedData.Domain != "" {
		// Check domain is in allow-list
		foundKey := false
		for _, k := range domains {
			if k.Key == digestData.SignedData.Domain {
				keyName = k.Name
				foundKey = true
				break
			}
		}
		if !foundKey {
			return nil, fmt.Errorf("wacz signer domain was not in allow-list")
		}

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

		verified, err = verifyDomainSignature(
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

	userAgent, err := findUserAgent(packageData, fileMap)
	if err != nil {
		log.Printf("failed to find user agent in data.warc.gz: %v", err)
	}

	return &WaczFileData{
		DigestData:  &digestData,
		PackageData: &packageData,
		UserAgent:   userAgent,
		KeyName:     keyName,
	}, nil
}

func GetVerifiedMetadata(filePath string, anonKeys, domains []*common.AllowedKey) (map[string]any, error) {
	mediaType := "application/wacz"
	metadata, err := ReadAndVerifyWaczMetadata(filePath, anonKeys, domains)
	if err != nil {
		return nil, err
	}
	var wacz map[string]string
	if metadata.DigestData.SignedData.PublicKey != "" {
		wacz = map[string](string){
			"hash":      metadata.DigestData.SignedData.Hash,
			"signature": metadata.DigestData.SignedData.Signature,
			"publicKey": metadata.DigestData.SignedData.PublicKey,
			"created":   metadata.PackageData.Created.UTC().Format(time.RFC3339),
			"software":  metadata.PackageData.Software,
		}
	} else {
		wacz = map[string](string){
			"hash":          metadata.DigestData.SignedData.Hash,
			"signature":     metadata.DigestData.SignedData.Signature,
			"version":       metadata.DigestData.SignedData.Version,
			"domain":        metadata.DigestData.SignedData.Domain,
			"domainCert":    metadata.DigestData.SignedData.DomainCert,
			"timeSignature": metadata.DigestData.SignedData.Signature,
			"timestampCert": metadata.DigestData.SignedData.TimestampCert,
			"created":       metadata.PackageData.Created.UTC().Format(time.RFC3339),
			"software":      metadata.PackageData.Software,
		}
	}

	// WACZs from Browsertrix don't seem to have the "modified" set, so replace it here
	modified := metadata.PackageData.Modified
	if modified.IsZero() {
		modified = metadata.PackageData.Created
	}

	waczMetadata := map[string]any{
		"last_modified":             modified,
		"time_created":              metadata.PackageData.Created,
		"media_type":                mediaType,
		"asset_origin_type":         []string{"wacz"},
		"crawl_user_agent":          metadata.UserAgent,
		"wacz":                      wacz,
		"asset_origin_sig_key_name": metadata.KeyName,
	}
	return waczMetadata, nil
}
