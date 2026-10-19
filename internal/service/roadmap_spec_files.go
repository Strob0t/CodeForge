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
	"github.com/Strob0t/CodeForge/internal/port/specprovider"
)

// Spec file import and write-back (KI-203). The roadmap records what it
// saw of a spec file each time it imports the file or writes its checkbox
// markers back: the content hash and the state of each imported feature's
// checkbox. So it knows whether the file changed since:
//   - an import takes a checkbox's state from the file only when that box
//     changed (importedStatus, a three-way merge);
//   - "Sync to file" writes only to a file that did not change at all.

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
// the hash the roadmap recorded for the file, and returns the boxes it
// recorded (nil without a record).
func (s *RoadmapService) specFileState(ctx context.Context, roadmapID, path, sum string) (specFileState, map[string]bool, error) {
	recorded, err := s.store.GetSpecFile(ctx, roadmapID, path)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return specFileUnrecorded, nil, nil
	case err != nil:
		return specFileUnrecorded, nil, err
	case recorded.ContentSHA256 == sum:
		return specFileUnchanged, recorded.Checked, nil
	default:
		return specFileChanged, recorded.Checked, nil
	}
}

// contentSHA256 is the hex SHA-256 of a spec file's content.
func contentSHA256(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// importedStatus is the status an imported checkbox gives its existing
// feature, and whether it differs from cur. It is a three-way merge with
// the box's state at the last import or sync (last, when seen): a box that
// did not change since keeps the roadmap's status, which is newer. A box
// that changed sets it: checked makes the feature done, unchecked reopens
// a done feature (other statuses are unchecked boxes too). Without a
// record of the box, a checked box makes the feature done but an unchecked
// one never reopens it: the roadmap may know of work the file never got.
func importedStatus(cur roadmap.FeatureStatus, checked, last, seen bool) (roadmap.FeatureStatus, bool) {
	switch {
	case seen && checked == last:
		return cur, false
	case checked && cur != roadmap.FeatureDone:
		return roadmap.FeatureDone, true
	case !checked && cur == roadmap.FeatureDone && seen:
		return roadmap.FeatureBacklog, true
	default:
		return cur, false
	}
}

// specItemPair is a checkbox item of a spec file and the feature it takes
// (nil for a new item).
type specItemPair struct {
	item    specprovider.SpecItemDetail
	feature *roadmap.Feature
}

// pairSpecItems pairs the checkbox items of the spec file path with the
// roadmap's features of that file, and returns the features no item takes.
// In a file unchanged since the last import or sync, an item takes the
// feature whose spec_ref names its line, whatever the feature's title is
// now (it may have been renamed in the UI; the hash proves the line did
// not move). The other items take features by title: the n-th item with a
// title the feature whose line comes n-th (fileFeaturesByTitle).
func pairSpecItems(items []specprovider.SpecItemDetail, features []roadmap.Feature, path string, unchanged bool) ([]specItemPair, []*roadmap.Feature) {
	byTitle := fileFeaturesByTitle(features, path)
	atLine := make(map[int]*roadmap.Feature)
	if unchanged {
		for _, same := range byTitle {
			for _, f := range same {
				if _, line := parseSpecRef(f.SpecRef); line > 0 {
					atLine[line] = f
				}
			}
		}
	}

	taken := make(map[*roadmap.Feature]bool)
	var pairs []specItemPair
	for _, item := range items {
		if item.Level != specItemCheckbox {
			continue
		}
		f := atLine[item.SourceLine]
		if f != nil {
			taken[f] = true
		}
		pairs = append(pairs, specItemPair{item: item, feature: f})
	}
	for i := range pairs {
		if pairs[i].feature != nil {
			continue
		}
		title := pairs[i].item.Title
		same := byTitle[title]
		for len(same) > 0 && taken[same[0]] {
			same = same[1:]
		}
		if len(same) > 0 {
			pairs[i].feature = same[0]
			taken[same[0]] = true
			same = same[1:]
		}
		byTitle[title] = same
	}

	var rest []*roadmap.Feature
	for _, same := range byTitle {
		for _, f := range same {
			if !taken[f] {
				rest = append(rest, f)
			}
		}
	}
	return pairs, rest
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
