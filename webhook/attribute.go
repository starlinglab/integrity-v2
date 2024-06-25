package webhook

import (
	"fmt"
	"slices"

	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/util"
)

// list of attributes that should be indexed as string
var indexedStringKeys = []string{"file_name", "asset_origin_id", "project_id"}

// ParseMapToAttributes parses a map and a file stat map
// to a slice of attributes for POSTing to the AA server
// It also reads the encryption keys from the filesystem,
// if fields are put under "private" key.
// Note that all keys under "private" are promoted to top level
// in encrypted form
func ParseMapToAttributes(cid string, attrMap map[string]any, fileAttributes map[string]any) ([]aa.PostKV, error) {

	var attributes []aa.PostKV

	for key, value := range attrMap {
		if key == "private" {
			privMap, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("private must be a map of private key-value pairs")
			}
			for pKey, pValue := range privMap {
				_, encKey, _, err := util.GenerateEncKey(cid, pKey)
				if err != nil {
					return nil, fmt.Errorf("error reading key: %w", err)
				}
				attributes = append(attributes, aa.PostKV{Key: pKey, Value: pValue, EncKey: encKey})
			}
		} else if slices.Contains(indexedStringKeys, key) {
			attributes = append(attributes, aa.PostKV{Key: key, Value: value, Type: "str"})
		} else {
			attributes = append(attributes, aa.PostKV{Key: key, Value: value})
		}
	}

	for key, value := range fileAttributes {
		attributes = append(attributes, aa.PostKV{Key: key, Value: value})
	}

	return attributes, nil
}
