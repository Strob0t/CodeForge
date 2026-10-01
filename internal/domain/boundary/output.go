package boundary

import (
	"encoding/json"
	"errors"
	"strings"
)

// ErrNoBoundariesJSON: the output holds no BOUNDARIES.json array.
var ErrNoBoundariesJSON = errors.New("no BOUNDARIES.json array (a JSON array of boundary objects) in the output")

// FromOutput reads the BOUNDARIES.json array from a boundary analyzer's
// output: the first JSON array in it, optionally in a code fence. The entries
// are decoded, not validated (see BoundaryFile.Validate); an empty array is a
// valid answer (no boundaries found).
func FromOutput(output string) ([]BoundaryFile, error) {
	start, end := strings.Index(output, "["), strings.LastIndex(output, "]")
	if start < 0 || end <= start {
		return nil, ErrNoBoundariesJSON
	}
	var entries []BoundaryFile
	if err := json.Unmarshal([]byte(output[start:end+1]), &entries); err != nil {
		return nil, ErrNoBoundariesJSON
	}
	return entries, nil
}
