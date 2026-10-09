package boundary

import (
	"encoding/json"
	"errors"
	"strings"
)

// ErrNoBoundariesJSON: the output holds no BOUNDARIES.json array.
var ErrNoBoundariesJSON = errors.New("no BOUNDARIES.json array (a JSON array of boundary objects) in the output")

const jsonFence = "```json"

// FromOutput reads the BOUNDARIES.json array from a boundary analyzer's
// output. A ```json code block that holds the array wins; otherwise the
// output is scanned from each '[' for a JSON array of boundary objects, so
// prose brackets before and after it ("[1]", "[a.go]") are skipped. The
// first non-empty array is taken, an empty one ("no boundaries found") only
// when there is none. The entries are decoded, not validated (see
// BoundaryFile.Validate).
func FromOutput(output string) ([]BoundaryFile, error) {
	for rest := output; ; {
		i := strings.Index(rest, jsonFence)
		if i < 0 {
			break
		}
		block := rest[i+len(jsonFence):]
		end := strings.Index(block, "```")
		if end < 0 {
			break
		}
		if entries, ok := decodeArray(strings.TrimSpace(block[:end])); ok {
			return entries, nil
		}
		rest = block[end+3:]
	}

	var empty []BoundaryFile
	for i := strings.IndexByte(output, '['); i >= 0; {
		if entries, ok := decodeArray(output[i:]); ok {
			if len(entries) > 0 {
				return entries, nil
			}
			if empty == nil {
				empty = entries
			}
		}
		next := strings.IndexByte(output[i+1:], '[')
		if next < 0 {
			break
		}
		i += 1 + next
	}
	if empty != nil {
		return empty, nil
	}
	return nil, ErrNoBoundariesJSON
}

// decodeArray decodes the JSON array of boundary objects at the start of s;
// what follows it is ignored.
func decodeArray(s string) ([]BoundaryFile, bool) {
	if !strings.HasPrefix(s, "[") {
		return nil, false
	}
	entries := []BoundaryFile{}
	if err := json.NewDecoder(strings.NewReader(s)).Decode(&entries); err != nil {
		return nil, false
	}
	return entries, true
}
