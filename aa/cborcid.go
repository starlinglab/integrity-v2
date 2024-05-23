package aa

import (
	"encoding/base32"
	"errors"
)

// CborCID is a CIDv1 as it is encoded in the DAG-CBOR format.
//
// This is not the same as just the bytes of a CIDv1.
// See https://ipld.io/specs/codecs/dag-cbor/spec/#links
// and https://github.com/ipld/cid-cbor
type CborCID []byte

var (
	// https://github.com/multiformats/multibase
	// The encoding referred to by "base32" or "b".
	// "RFC4648 case-insensitive - no padding"
	multibaseBase32 = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)
)

// NewCborCID takes in a base32 CIDv1 and returns a CborCID, valid for encoding CIDs in DAG-CBOR.
//
// The input CID string is not fully validated and so it is possible for this function to
// output invalid values.
func NewCborCID(cid string) (CborCID, error) {
	if cid[0] != 'b' {
		return nil, errors.New("only base32-encoding CIDs accepted")
	}
	// Turn into binary, change multibase prefix from base32 "b" to 0x00
	bin, err := multibaseBase32.DecodeString(cid[1:])
	if err != nil {
		return nil, err
	}
	return CborCID(append([]byte{0x00}, bin...)), nil
}

// String returns the CborCID as a standard base32 CIDv1 string.
// This fulfills the fmt.Stringer interface, useful for fmt.Print and similar.
func (c CborCID) String() string {
	// Change multibase prefix from 0x00 to "b" for base32, and convert to base32
	return "b" + multibaseBase32.EncodeToString(c[1:])
}

// MarshalJSON fulfills the json.Marshaler interface.
//
// It looks like CID(bafy...) which indicates that the value was originally stored in binary
// rather than a native JSON or string encoding.
func (c CborCID) MarshalJSON() (text []byte, err error) {
	// Subtract 1 due to first byte (0x00) not being encoded
	// Add 2 for ending bytes of text repr. `)"`
	enc := make([]byte, multibaseBase32.EncodedLen(len(c)-1)+2)
	multibaseBase32.Encode(enc, c[1:])
	enc[len(enc)-2] = ')'
	enc[len(enc)-1] = '"'
	text = append([]byte(`"CID(b`), enc...)
	return
}
