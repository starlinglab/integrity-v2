package webhook

import (
	"fmt"

	"github.com/starlinglab/integrity-v2/aa"
	"github.com/starlinglab/integrity-v2/util"
)

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
			if _, ok := value.(map[string]any); ok {
				// private is a map
				for pKey, pValue := range value.(map[string]any) {
					_, encKey, _, err := util.GenerateEncKey(cid, pKey)
					if err != nil {
						return nil, fmt.Errorf("error reading key: %w", err)
					}
					attributes = append(attributes, aa.PostKV{Key: pKey, Value: pValue, EncKey: encKey})
				}
			} else {
				// private is a value
				_, encKey, _, err := util.GenerateEncKey(cid, key)
				if err != nil {
					return nil, fmt.Errorf("error reading key: %w", err)
				}
				attributes = append(attributes, aa.PostKV{Key: key, Value: value, EncKey: encKey})
			}
		} else {
			attributes = append(attributes, aa.PostKV{Key: key, Value: value})
		}
	}

	for key, value := range fileAttributes {
		attributes = append(attributes, aa.PostKV{Key: key, Value: value})
	}

	return attributes, nil
}
