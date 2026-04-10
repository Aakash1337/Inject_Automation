package evidence

import "encoding/json"

func marshalJSON(payload any) ([]byte, error) {
	return json.MarshalIndent(payload, "", "  ")
}
