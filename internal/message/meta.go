package message

import "encoding/json"

func normalizeMeta(meta map[string]any) map[string]any {
	if meta == nil {
		return map[string]any{}
	}
	return meta
}

func marshalMeta(meta map[string]any) ([]byte, error) {
	return json.Marshal(normalizeMeta(meta))
}

func unmarshalMeta(data []byte) map[string]any {
	if len(data) == 0 {
		return map[string]any{}
	}
	var meta map[string]any
	if err := json.Unmarshal(data, &meta); err != nil || meta == nil {
		return map[string]any{}
	}
	return meta
}
