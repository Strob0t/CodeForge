package service

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/roadmap"
)

// Spec file import and write-back (KI-203). The roadmap records the content
// hash of a spec file each time it imports the file or writes its checkbox
// markers back, so it knows whether the file changed since:
//   - an import takes a checkbox's state from the file only when it did
//     (importedStatus);
//   - "Sync to file" writes only to a file that did not.

// Values of specprovider.SpecItemDetail's Level and Status.
const (
	specItemCheckbox = "checkbox"
	specItemDone     = "done"
)

// specFileState is how a spec file's content relates to the content the
// roadmap last imported or wrote back.
type specFileState int

const (
	// specFileUnrecorded: no record, the file was imported before KI-203
	// (or never).
	specFileUnrecorded specFileState = iota
	specFileUnchanged
	specFileChanged
)

// specFileState compares sum, the hash of a spec file's content now, with
// the hash the roadmap recorded for the file.
func (s *RoadmapService) specFileState(ctx context.Context, roadmapID, path, sum string) (specFileState, error) {
	recorded, err := s.store.GetSpecFileHash(ctx, roadmapID, path)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return specFileUnrecorded, nil
	case err != nil:
		return specFileUnrecorded, err
	case recorded == sum:
		return specFileUnchanged, nil
	default:
		return specFileChanged, nil
	}
}

// contentSHA256 is the hex SHA-256 of a spec file's content.
func contentSHA256(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// importedStatus is the status an imported checkbox gives its existing
// feature, and whether it differs from cur. When the file is unchanged
// since the last import or sync, the roadmap's status is newer and stays.
// When it changed, the file's state wins: a checked box makes the feature
// done, an unchecked one reopens a done feature (other statuses are
// unchecked boxes too). Without a record, a checked box makes the feature
// done but an unchecked one never reopens it: the roadmap may know of work
// the file never got.
func importedStatus(cur roadmap.FeatureStatus, checked bool, state specFileState) (roadmap.FeatureStatus, bool) {
	switch {
	case state == specFileUnchanged:
		return cur, false
	case checked && cur != roadmap.FeatureDone:
		return roadmap.FeatureDone, true
	case !checked && cur == roadmap.FeatureDone && state == specFileChanged:
		return roadmap.FeatureBacklog, true
	default:
		return cur, false
	}
}

// fileFeaturesByTitle groups the features whose spec_ref names the spec
// file path (with or without a line) by title, each group in file order
// (features without a line last).
func fileFeaturesByTitle(features []roadmap.Feature, path string) map[string][]*roadmap.Feature {
	byTitle := make(map[string][]*roadmap.Feature)
	for i := range features {
		if specRefFile(features[i].SpecRef) == path {
			byTitle[features[i].Title] = append(byTitle[features[i].Title], &features[i])
		}
	}
	for _, same := range byTitle {
		slices.SortStableFunc(same, func(a, b *roadmap.Feature) int {
			return cmp.Compare(refLineOrLast(a.SpecRef), refLineOrLast(b.SpecRef))
		})
	}
	return byTitle
}

// refLineOrLast is the line of a "path#Lline" spec_ref, or the largest int
// for a spec_ref without one.
func refLineOrLast(specRef string) int {
	if _, line := parseSpecRef(specRef); line > 0 {
		return line
	}
	return math.MaxInt
}

// parseSpecRef extracts the file path and line number from a spec_ref
// in the format "path/to/file.md#L42". Returns ("", 0) if the format
// is not recognized.
func parseSpecRef(specRef string) (filePath string, line int) {
	idx := strings.LastIndex(specRef, "#L")
	if idx < 0 {
		return "", 0
	}
	n, err := strconv.Atoi(specRef[idx+len("#L"):])
	if err != nil || n < 1 {
		return "", 0
	}
	return specRef[:idx], n
}

// specRefFile is the file a spec_ref names: the path of "path#Lline", or
// the whole spec_ref.
func specRefFile(specRef string) string {
	if path, line := parseSpecRef(specRef); line > 0 {
		return path
	}
	return specRef
}
