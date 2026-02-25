package knowledgebase

import (
	"testing"
)

func TestCreateRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateRequest
		wantErr bool
	}{
		{
			name:    "empty name",
			req:     CreateRequest{Name: "", Category: "framework"},
			wantErr: true,
		},
		{
			name:    "invalid category",
			req:     CreateRequest{Name: "test", Category: "invalid"},
			wantErr: true,
		},
		{
			name:    "valid request",
			req:     CreateRequest{Name: "test-kb", Category: "framework", Description: "Test KB"},
			wantErr: false,
		},
		{
			name:    "valid custom category",
			req:     CreateRequest{Name: "my-kb", Category: "custom"},
			wantErr: false,
		},
		{
			name:    "all categories valid",
			req:     CreateRequest{Name: "sec-kb", Category: "security"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidCategories(t *testing.T) {
	valid := []Category{
		CategoryFramework,
		CategoryParadigm,
		CategoryLanguage,
		CategorySecurity,
		CategoryCustom,
	}
	for _, c := range valid {
		req := CreateRequest{Name: "test", Category: c}
		if err := req.Validate(); err != nil {
			t.Errorf("expected category %q to be valid, got error: %v", c, err)
		}
	}
}
