package trx

import "encoding/json"

func ObjectToMap(obj any) (map[string]any, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	err = json.Unmarshal(data, &m)
	return m, err
}

func MapToObject(m map[string]any, obj any) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, obj)
}
