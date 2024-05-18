package utils

type KeyValuePair struct {
	key   string
	value interface{}
}

func ParseJsonToAttributes(jsonMap interface{}) []KeyValuePair {

	var attributes []KeyValuePair

	parsedMap, ok := jsonMap.(map[string]interface{})
	if ok {
		contentMetadata, ok := parsedMap["contentMetadata"]
		if ok {
			contentMetadataMap, ok := contentMetadata.(map[string]interface{})
			if ok {
				for k, v := range contentMetadataMap {
					if k != "private" {
						attributes = append(attributes, KeyValuePair{key: k, value: v})
					}
				}
			}
		}
	}
	return attributes
}
