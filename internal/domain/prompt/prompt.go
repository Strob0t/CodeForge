package prompt

// SectionRow represents a prompt section stored in the database.
type SectionRow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Scope     string `json:"scope"` // "global", "mode:{id}", "project:{id}"
	Content   string `json:"content"`
	Priority  int    `json:"priority"`
	SortOrder int    `json:"sort_order"`
	Enabled   bool   `json:"enabled"`
	Merge     string `json:"merge"` // "replace", "prepend", "append"
}
