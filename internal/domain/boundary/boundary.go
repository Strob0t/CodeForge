package boundary

import (
	"errors"
	"strconv"
	"time"
)

// BoundaryType identifies the kind of contract boundary a file represents.
type BoundaryType string

const (
	BoundaryTypeAPI           BoundaryType = "api"
	BoundaryTypeData          BoundaryType = "data"
	BoundaryTypeInterService  BoundaryType = "inter-service"
	BoundaryTypeCrossLanguage BoundaryType = "cross-language"
)

var validBoundaryTypes = map[BoundaryType]bool{
	BoundaryTypeAPI:           true,
	BoundaryTypeData:          true,
	BoundaryTypeInterService:  true,
	BoundaryTypeCrossLanguage: true,
}

// Validate returns an error if the BoundaryType is not one of the recognised values.
func (bt BoundaryType) Validate() error {
	if !validBoundaryTypes[bt] {
		return errors.New("invalid boundary type: " + string(bt))
	}
	return nil
}

// BoundaryFile is a single file that forms (or participates in) a contract boundary.
type BoundaryFile struct {
	Path         string       `json:"path"`
	Type         BoundaryType `json:"type"`
	Counterpart  string       `json:"counterpart,omitempty"`
	AutoDetected bool         `json:"auto_detected"`
}

// Validate returns an error if the BoundaryFile has an empty path or an invalid type.
func (bf BoundaryFile) Validate() error {
	if bf.Path == "" {
		return errors.New("boundary file path must not be empty")
	}
	return bf.Type.Validate()
}

// ProjectBoundaryConfig holds the full boundary configuration for a project.
type ProjectBoundaryConfig struct {
	ProjectID    string         `json:"project_id"`
	TenantID     string         `json:"tenant_id"`
	Boundaries   []BoundaryFile `json:"boundaries"`
	LastAnalyzed time.Time      `json:"last_analyzed"`
	Version      int            `json:"version"`
}

// Validate returns an error if the config is missing a project ID or contains
// any invalid BoundaryFile entries.
func (c *ProjectBoundaryConfig) Validate() error {
	if c.ProjectID == "" {
		return errors.New("project_id must not be empty")
	}
	for i, bf := range c.Boundaries {
		if err := bf.Validate(); err != nil {
			return errors.New("boundary[" + strconv.Itoa(i) + "]: " + err.Error())
		}
	}
	return nil
}
