package proofmode

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	"lukechampine.com/blake3"

	"github.com/starlinglab/integrity-v2/preprocessor/common"
)

type ProofModeAssetMetadata struct {
	Sha256         string `json:"File Hash SHA256"`
	FilePath       string `json:"File Path"`
	FileModified   string `json:"File Modified"`
	FileCreated    string `json:"File Created"`
	ProofGenerated string `json:"Proof Generated"`
	Note           string `json:"Note"`
}

type ProofModeFileData struct {
	AssetSignature    []byte
	MetadataBytes     []byte
	MetadataSignature []byte
	PubKey            []byte
	Ots               []byte
	Gst               []byte
	Sha256            string
	Md5               string
	Blake3            string
	FileSize          uint64
	MediaType         string
	Metadata          *ProofModeAssetMetadata
	KeyName           string
}

// validateAndParseProofModeFileSignatures reads a file and verify
// its asset and metadata hash and signature
func validateAndParseFileSignatures(
	fileMap map[string]*zip.File,
	fileName string,
	fileSha string,
	jsonMetadataBytes []byte,
	keys []*common.AllowedKey,
) (*ProofModeFileData, error) {

	openFile := func(m map[string]*zip.File, n string) (io.ReadCloser, error) {
		zipF, ok := m[n]
		if !ok {
			// Expected file not in ZIP
			return nil, fmt.Errorf("missing file: " + n)
		}
		return zipF.Open()
	}

	// read key
	keyFile, err := openFile(fileMap, "pubkey.asc")
	if err != nil {
		return nil, err
	}
	defer keyFile.Close()
	keyFileBytes, err := io.ReadAll(keyFile)
	if err != nil {
		return nil, err
	}
	keyRing, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(keyFileBytes))
	if err != nil {
		return nil, err
	}

	// Check key fingerprint against allow-list
	fp := keyRing[0].PrimaryKey.KeyIdString()
	var keyName string
	foundKey := false
	for _, k := range keys {
		if len(k.Key) == 18 {
			k.Key = strings.TrimPrefix(k.Key, "0x")
		}
		if strings.ToUpper(k.Key) == fp {
			foundKey = true
			keyName = k.Name
			break
		}
	}
	if !foundKey {
		return nil, fmt.Errorf("proofmode public key was not on allow-list")
	}

	// read asset signature
	assetSignature, err := openFile(fileMap, fileSha+".asc")
	if err != nil {
		return nil, err
	}
	defer assetSignature.Close()
	assetSignatureBytes, err := io.ReadAll(assetSignature)
	if err != nil {
		return nil, err
	}

	// read asset
	assetFile, err := openFile(fileMap, fileName)
	if err != nil {
		return nil, err
	}
	headerBytes := make([]byte, 512)
	_, err = io.ReadFull(assetFile, headerBytes)
	if err != nil {
		return nil, err
	}
	mediaType := http.DetectContentType(headerBytes)
	assetFile.Close()

	assetFile, err = openFile(fileMap, fileName)
	if err != nil {
		return nil, err
	}
	defer assetFile.Close()

	fileSize := fileMap[fileName].UncompressedSize64
	sha := sha256.New()
	md := md5.New()
	blake := blake3.New(32, nil)
	assetFileReader := io.TeeReader(assetFile, io.MultiWriter(sha, md, blake))
	// verify asset signature
	_, err = openpgp.CheckArmoredDetachedSignature(keyRing, assetFileReader, bytes.NewReader(assetSignatureBytes), nil)
	if err != nil {
		return nil, fmt.Errorf("asset signature verification failed")
	}

	// verify asset sha
	shaSum := hex.EncodeToString(sha.Sum(nil))
	if shaSum != fileSha {
		return nil, fmt.Errorf("file hash mismatch")
	}

	// verify json metadata signature
	jsonMetadataSignature, err := openFile(fileMap, fileSha+".proof.json.asc")
	if err != nil {
		return nil, err
	}
	defer jsonMetadataSignature.Close()
	jsonMetadataSignatureBytes, err := io.ReadAll(jsonMetadataSignature)
	if err != nil {
		return nil, err
	}
	_, err = openpgp.CheckArmoredDetachedSignature(keyRing, bytes.NewReader(jsonMetadataBytes), bytes.NewReader(jsonMetadataSignatureBytes), nil)
	if err != nil {
		return nil, fmt.Errorf("metadata signature verification failed")
	}

	// verify csv metadata signature
	csvMetadata, err := openFile(fileMap, fileSha+".proof.csv")
	if err != nil {
		return nil, err
	}
	defer csvMetadata.Close()
	csvMetadataBytes, err := io.ReadAll(csvMetadata)
	if err != nil {
		return nil, err
	}

	csvMetadataSignature, err := openFile(fileMap, fileSha+".proof.csv.asc")
	if err != nil {
		return nil, err
	}
	defer csvMetadataSignature.Close()
	csvMetadataSignatureBytes, err := io.ReadAll(csvMetadataSignature)
	if err != nil {
		return nil, err
	}
	_, err = openpgp.CheckArmoredDetachedSignature(keyRing, bytes.NewReader(csvMetadataBytes), bytes.NewReader(csvMetadataSignatureBytes), nil)
	if err != nil {
		return nil, fmt.Errorf("metadata signature verification failed")
	}

	otsFile, err := openFile(fileMap, fileSha+".ots")
	if err != nil {
		return nil, err
	}
	defer otsFile.Close()
	otsBytes, err := io.ReadAll(otsFile)
	if err != nil {
		return nil, err
	}

	gstFile, err := openFile(fileMap, fileSha+".gst")
	if err != nil {
		return nil, err
	}
	defer gstFile.Close()
	gstBytes, err := io.ReadAll(gstFile)
	if err != nil {
		return nil, err
	}

	fileData := ProofModeFileData{
		Sha256:            hex.EncodeToString(sha.Sum(nil)),
		Md5:               hex.EncodeToString(md.Sum(nil)),
		Blake3:            hex.EncodeToString(blake.Sum(nil)),
		AssetSignature:    assetSignatureBytes,
		MetadataBytes:     jsonMetadataBytes,
		MetadataSignature: jsonMetadataSignatureBytes,
		PubKey:            keyFileBytes,
		Ots:               otsBytes,
		Gst:               gstBytes,
		FileSize:          fileSize,
		MediaType:         mediaType,
		Metadata:          nil,
		KeyName:           keyName,
	}

	return &fileData, nil
}

// parseProofModeBundleAssetInfo reads the files in the zip
// and returns a map of files and the json metadata files
func parseBundleAssetInfo(zipReader *zip.ReadCloser) (map[string]*zip.File, [][]byte, error) {
	var jsonFilesBytes [][]byte
	fileMap := map[string]*zip.File{}
	for _, file := range zipReader.File {
		if filepath.Ext(file.FileInfo().Name()) == ".json" {
			jsonFileReader, err := file.Open()
			if err != nil {
				return nil, nil, err
			}
			defer jsonFileReader.Close()
			jsonFileBytes, err := io.ReadAll(jsonFileReader)
			if err != nil {
				return nil, nil, err
			}
			jsonFilesBytes = append(jsonFilesBytes, jsonFileBytes)
		} else {
			fileMap[file.Name] = file
		}
	}
	return fileMap, jsonFilesBytes, nil
}

// CheckIsProofModeFile checks if the file is a proofmode file
func CheckIsProofModeFile(filePath string) bool {
	if filepath.Ext(filePath) != ".zip" {
		return false
	}
	zipListing, err := zip.OpenReader(filePath)
	if err != nil {
		return false
	}
	defer zipListing.Close()
	found := false
	for _, file := range zipListing.File {
		if file.Name == "HowToVerifyProofData.txt" {
			found = true
			break
		}
	}
	return found
}

// GetMapOfZipFiles reads the files mapping and JSON metadata in the zip
func GetMapOfZipFiles(zipListing *zip.ReadCloser) (map[string]*zip.File, [][]byte, error) {
	fileMap, jsonFilesBytes, err := parseBundleAssetInfo(zipListing)
	if err != nil {
		return nil, nil, err
	}
	if len(jsonFilesBytes) == 0 {
		return nil, nil, fmt.Errorf("missing json metadata file")
	}
	return fileMap, jsonFilesBytes, nil
}

// ReadAndVerifyProofModeMetadata reads and verifies a proof mode file
// and returns its metadata
func ReadAndVerifyMetadata(filePath string, keys []*common.AllowedKey) ([]*ProofModeFileData, error) {
	zipListing, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, err
	}
	defer zipListing.Close()
	fileMap, jsonFilesBytes, err := GetMapOfZipFiles(zipListing)
	if err != nil {
		return nil, err
	}

	var ProofModeFileDatas []*ProofModeFileData
	for _, jsonFileBytes := range jsonFilesBytes {
		var metadata ProofModeAssetMetadata
		err = json.Unmarshal(jsonFileBytes, &metadata)
		if err != nil {
			return nil, err
		}
		filename := filepath.Base(metadata.FilePath)
		assetData, err := validateAndParseFileSignatures(
			fileMap, filename, metadata.Sha256, jsonFileBytes, keys,
		)
		if err != nil {
			return nil, err
		}
		assetData.Metadata = &metadata
		ProofModeFileDatas = append(ProofModeFileDatas, assetData)
	}

	return ProofModeFileDatas, nil
}
