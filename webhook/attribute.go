package webhook

import (
	"github.com/starlinglab/integrity-v2/aa"
)

func CastMapForJSON(originalMap map[any]any) map[string]any {
	newMap := make(map[string]any)
	// handle value is array
	for key, value := range originalMap {
		if nestedArray, ok := value.([]any); ok {
			stringKey := key.(string)
			newMap[stringKey] = []any{}
			for _, v := range nestedArray {
				if nestedMap, ok := v.(map[any]any); ok {
					newMap[stringKey] = append(newMap[stringKey].([]any), CastMapForJSON(nestedMap))
				} else {
					newMap[stringKey] = append(newMap[stringKey].([]any), v)
				}
			}
			// handle value is map
		} else if nestedMap, ok := value.(map[any]any); ok {
			newMap[key.(string)] = CastMapForJSON(nestedMap)
		} else {
			newMap[key.(string)] = value
		}
	}
	return newMap
}

func ParseJsonToAttributes(jsonMap map[string]any) []aa.AttributeKeyValuePair {

	var attributes []aa.AttributeKeyValuePair

	for k, v := range jsonMap {
		// TODO: add whitelist/blacklist for attributes in config
		if k != "private" {
			attributes = append(attributes, aa.AttributeKeyValuePair{Key: k, Value: v})
		}
	}

	return attributes
}
