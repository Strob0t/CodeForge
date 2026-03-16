package boundary

import "testing"

func TestBoundaryTypeValid(t *testing.T) {
	tests := []struct {
		name    string
		bt      BoundaryType
		wantErr bool
	}{
		{"api", BoundaryTypeAPI, false},
		{"data", BoundaryTypeData, false},
		{"inter-service", BoundaryTypeInterService, false},
		{"cross-language", BoundaryTypeCrossLanguage, false},
		{"empty", BoundaryType(""), true},
		{"invalid", BoundaryType("foobar"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.bt.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBoundaryFileValidate(t *testing.T) {
	tests := []struct {
		name    string
		bf      BoundaryFile
		wantErr bool
	}{
		{"valid", BoundaryFile{Path: "api/schema.proto", Type: BoundaryTypeAPI}, false},
		{"with counterpart", BoundaryFile{Path: "models.py", Type: BoundaryTypeData, Counterpart: "types.ts"}, false},
		{"empty path", BoundaryFile{Path: "", Type: BoundaryTypeAPI}, true},
		{"invalid type", BoundaryFile{Path: "foo.go", Type: BoundaryType("bad")}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.bf.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProjectBoundaryConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ProjectBoundaryConfig
		wantErr bool
	}{
		{"valid", ProjectBoundaryConfig{
			ProjectID:  "proj-1",
			Boundaries: []BoundaryFile{{Path: "a.proto", Type: BoundaryTypeAPI}},
		}, false},
		{"empty project id", ProjectBoundaryConfig{ProjectID: ""}, true},
		{"nil boundaries ok", ProjectBoundaryConfig{ProjectID: "proj-1"}, false},
		{"empty tenant id", ProjectBoundaryConfig{ProjectID: "proj-1", TenantID: ""}, false},
		{"invalid boundary", ProjectBoundaryConfig{
			ProjectID:  "proj-1",
			Boundaries: []BoundaryFile{{Path: "", Type: BoundaryTypeAPI}},
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
