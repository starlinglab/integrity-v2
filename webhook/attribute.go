package webhook

import (
	"fmt"
	"slices"

	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/util"
)

// fields that are marked as private
var privateFields = []string{"private", "proofmode"}

// ParseMapToAttributes parses a map and a file stat map
// to a slice of attributes for POSTing to the AA server
// It also reads the encryption keys from the filesystem,
// if fields are marked as private
func ParseMapToAttributes(cid string, attrMap map[string]any, fileAttributes map[string]any) ([]aa.PostKV, error) {

	var attributes []aa.PostKV

	for k, v := range attrMap {
		// TODO: add whitelist/blacklist for attributes in config

		var encKey []byte
		if slices.Contains(privateFields, k) {
			_, keyBytes, _, err := util.GenerateEncKey(cid, k)
			if err != nil {
				return nil, fmt.Errorf("error reading key: %w", err)
			}
			encKey = keyBytes
		}
		attributes = append(attributes, aa.PostKV{Key: k, Value: v, EncKey: encKey})
	}

	for k, v := range fileAttributes {
		attributes = append(attributes, aa.PostKV{Key: k, Value: v})
	}

	return attributes, nil
}
