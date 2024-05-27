package util

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"path/filepath"

	"lukechampine.com/blake3"

	"golang.org/x/crypto/openpgp" // nolint:staticcheck
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
	Metadata          *ProofModeAssetMetadata
}

func validateAndParseProofModeFileSignatures(fileMap map[string]*zip.File, fileName string, fileSha string, jsonMetadataBytes []byte) (
	*ProofModeFileData,
	error,
) {
	// read key
	keyFile, err := fileMap["pubkey.asc"].Open()
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

	// read asset signature
	assetSignature, err := fileMap[fileSha+".asc"].Open()
	if err != nil {
		return nil, err
	}
	defer assetSignature.Close()
	assetSignatureBytes, err := io.ReadAll(assetSignature)
	if err != nil {
		return nil, err
	}

	// read asset
	assetFile, err := fileMap[fileName].Open()
	if err != nil {
		return nil, err
	}
	defer assetFile.Close()
	fileSize := fileMap[fileName].UncompressedSize64
	sha := sha256.New()
	md := md5.New()
	blake := blake3.New(32, nil)
	assetFileReader := io.TeeReader(assetFile, sha)
	assetFileReader = io.TeeReader(assetFileReader, md)
	assetFileReader = io.TeeReader(assetFileReader, blake)

	// verify asset signature
	_, err = openpgp.CheckArmoredDetachedSignature(keyRing, assetFileReader, bytes.NewReader(assetSignatureBytes))
	if err != nil {
		return nil, fmt.Errorf("asset signature verification failed")
	}

	// verify asset sha
	shaSum := hex.EncodeToString(sha.Sum(nil))
	if shaSum != fileSha {
		return nil, fmt.Errorf("file hash mismatch")
	}

	// verify json metadata signature
	jsonMetadataSignature, err := fileMap[fileSha+".proof.json.asc"].Open()
	if err != nil {
		return nil, err
	}
	defer jsonMetadataSignature.Close()
	jsonMetadataSignatureBytes, err := io.ReadAll(jsonMetadataSignature)
	if err != nil {
		return nil, err
	}
	_, err = openpgp.CheckArmoredDetachedSignature(keyRing, bytes.NewReader(jsonMetadataBytes), bytes.NewReader(jsonMetadataSignatureBytes))
	if err != nil {
		return nil, fmt.Errorf("metadata signature verification failed")
	}

	// verify csv metadata signature
	csvMetadata, err := fileMap[fileSha+".proof.csv"].Open()
	if err != nil {
		return nil, err
	}
	defer csvMetadata.Close()
	csvMetadataBytes, err := io.ReadAll(csvMetadata)
	if err != nil {
		return nil, err
	}

	csvMetadataSignature, err := fileMap[fileSha+".proof.csv.asc"].Open()
	if err != nil {
		return nil, err
	}
	defer csvMetadataSignature.Close()
	csvMetadataSignatureBytes, err := io.ReadAll(csvMetadataSignature)
	if err != nil {
		return nil, err
	}
	_, err = openpgp.CheckArmoredDetachedSignature(keyRing, bytes.NewReader(csvMetadataBytes), bytes.NewReader(csvMetadataSignatureBytes))
	if err != nil {
		return nil, fmt.Errorf("metadata signature verification failed")
	}

	otsFile, err := fileMap[fileSha+".ots"].Open()
	if err != nil {
		return nil, err
	}
	defer otsFile.Close()
	otsBytes, err := io.ReadAll(otsFile)
	if err != nil {
		return nil, err
	}

	gstFile, err := fileMap[fileSha+".gst"].Open()
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
		Metadata:          nil,
	}

	return &fileData, nil
}

func parseProofModeBundleAssetInfo(zipReader *zip.ReadCloser) (map[string]*zip.File, [][]byte, error) {
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

func CheckIsProofModeFile(filePath string) bool {
	if filepath.Ext(filePath) != ".zip" {
		return false
	}
	zipListing, err := zip.OpenReader(filePath)
	if err != nil {
		return false
	}
	found := false
	for _, file := range zipListing.File {
		if file.Name == "HowToVerifyProofData.txt" {
			found = true
			break
		}
	}
	return found
}

func GetProofModeZipFiles(zipListing *zip.ReadCloser) (map[string]*zip.File, [][]byte, error) {
	fileMap, jsonFilesBytes, err := parseProofModeBundleAssetInfo(zipListing)
	if err != nil {
		return nil, nil, err
	}
	if len(jsonFilesBytes) == 0 {
		return nil, nil, fmt.Errorf("missing json metadata file")
	}
	return fileMap, jsonFilesBytes, nil
}

func ReadAndVerifyProofModeMetadata(filePath string) ([]*ProofModeFileData, error) {
	zipListing, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, err
	}
	defer zipListing.Close()
	fileMap, jsonFilesBytes, err := GetProofModeZipFiles(zipListing)
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
		assetData, err := validateAndParseProofModeFileSignatures(fileMap, filename, metadata.Sha256, jsonFileBytes)
		if err != nil {
			return nil, err
		}
		assetData.Metadata = &metadata
		ProofModeFileDatas = append(ProofModeFileDatas, assetData)
	}

	return ProofModeFileDatas, nil
}
