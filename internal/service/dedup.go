package service

import (
	"hash/fnv"
	"math/bits"
	"sort"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
)

// simhash64 computes a 64-bit SimHash fingerprint for text.
// SimHash works by: 1) tokenizing into 3-character shingles (trigrams),
// 2) hashing each shingle with FNV-64a, 3) accumulating weighted bit vectors,
// 4) thresholding to binary. Returns 0 for texts shorter than 3 bytes.
func simhash64(text string) uint64 {
	if len(text) < 3 {
		return 0
	}

	var v [64]int
	for i := 0; i <= len(text)-3; i++ {
		shingle := text[i : i+3]
		h := fnv.New64a()
		_, _ = h.Write([]byte(shingle))
		hash := h.Sum64()
		for j := 0; j < 64; j++ {
			if hash&(1<<uint(j)) != 0 {
				v[j]++
			} else {
				v[j]--
			}
		}
	}

	var fingerprint uint64
	for j := 0; j < 64; j++ {
		if v[j] > 0 {
			fingerprint |= 1 << uint(j)
		}
	}
	return fingerprint
}

// hammingDistance returns the number of differing bits between two uint64 values.
func hammingDistance(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}

// defaultDedupThreshold is the default SimHash hamming distance threshold
// for considering two context candidates as near-duplicates.
// 3 bits out of 64 corresponds to ~95% similarity.
const defaultDedupThreshold = 3

// deduplicateCandidates removes near-duplicate context candidates using SimHash.
// Two candidates are considered near-duplicates if their hamming distance is <= threshold.
// When duplicates are found, the one with the higher priority is kept.
// If threshold < 0, the default of 3 is used. Threshold 0 means exact match only.
func deduplicateCandidates(candidates []cfcontext.ContextEntry, threshold int) []cfcontext.ContextEntry {
	if len(candidates) == 0 {
		return candidates
	}

	if threshold < 0 {
		threshold = defaultDedupThreshold
	}

	type fingerprinted struct {
		candidate cfcontext.ContextEntry
		hash      uint64
	}

	fps := make([]fingerprinted, len(candidates))
	for i, c := range candidates {
		fps[i] = fingerprinted{
			candidate: c,
			hash:      simhash64(c.Content),
		}
	}

	// Sort by priority descending so higher-scored candidates are kept.
	sort.Slice(fps, func(i, j int) bool {
		return fps[i].candidate.Priority > fps[j].candidate.Priority
	})

	result := make([]cfcontext.ContextEntry, 0, len(fps))
	seen := make([]uint64, 0, len(fps))

	for _, fp := range fps {
		isDuplicate := false
		for _, seenHash := range seen {
			if hammingDistance(fp.hash, seenHash) <= threshold {
				isDuplicate = true
				break
			}
		}
		if !isDuplicate {
			result = append(result, fp.candidate)
			seen = append(seen, fp.hash)
		}
	}

	return result
}
