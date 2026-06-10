package text

import (
	"encoding/json"

	"github.com/user/miniweb/internal/minidom"
)

// EncodeJSON serializes a PageSnapshot to MiniDOM Text (JSON) format.
func EncodeJSON(snap *minidom.PageSnapshot) ([]byte, error) {
	snap.Format = "minidom-text"
	snap.Version = 1
	return json.Marshal(snap)
}

// DecodeJSON deserializes MiniDOM Text JSON into a PageSnapshot.
func DecodeJSON(data []byte) (*minidom.PageSnapshot, error) {
	var snap minidom.PageSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}
