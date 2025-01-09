package util

import (
	"crypto/sha256"
	"encoding/base32"
	"io"
)

var (
	// https://github.com/multiformats/multibase
	// The encoding referred to by "base32" or "b".
	// "RFC4648 case-insensitive - no padding"
	multibaseBase32 = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)
)

// CalculateFileCid gets the CIDv1 for the given data, using raw SHA-256.
// It does not load the whole file into memory.
func CalculateFileCid(fileReader io.Reader) (string, error) {
	hasher := sha256.New()
	_, err := io.Copy(hasher, fileReader)
	if err != nil {
		return "", err
	}
	// The bytes are (in order) CID version, raw multicodec, sha2-256 multihash, 32 byte length hash
	return "b" + multibaseBase32.EncodeToString(append([]byte{0x01, 0x55, 0x12, 0x20}, hasher.Sum(nil)...)), nil
}
