package store

import (
	"encoding/json"

	"github.com/thegliffy/2007mmo/internal/protocol"
)

// marshalLooks writes a finished face as JSON. Unset stays SQL NULL so
// "has the creator run" is a missing column, not an empty object.
func marshalLooks(l protocol.Looks) ([]byte, error) {
	if !l.Set() {
		return nil, nil
	}
	return json.Marshal(l)
}

func unmarshalLooks(raw []byte, dest *protocol.Looks) error {
	if len(raw) == 0 || dest == nil {
		return nil
	}
	return json.Unmarshal(raw, dest)
}
