package project

import "maps"

// ConfigPatch updates a project config with merge semantics: a key with a
// value is set, a key with a null value is removed, and keys the patch does
// not name are kept.
type ConfigPatch map[string]*string

// Apply returns current with the patch applied. current is not modified.
func (p ConfigPatch) Apply(current map[string]string) map[string]string {
	merged := make(map[string]string, len(current)+len(p))
	maps.Copy(merged, current)
	for key, value := range p {
		if value == nil {
			delete(merged, key)
			continue
		}
		merged[key] = *value
	}
	return merged
}
