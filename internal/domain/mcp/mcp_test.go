package mcp

import (
	"errors"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
)

func TestServerDef_Validate(t *testing.T) {
	tests := []struct {
		name    string
		def     ServerDef
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid stdio server",
			def: ServerDef{
				Name:      "test-server",
				Transport: TransportStdio,
				Command:   "/usr/bin/mcp-server",
				Args:      []string{"--port", "3000"},
			},
			wantErr: false,
		},
		{
			name: "valid sse server",
			def: ServerDef{
				Name:      "remote-server",
				Transport: TransportSSE,
				URL:       "http://localhost:8080/sse",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			def: ServerDef{
				Transport: TransportStdio,
				Command:   "/usr/bin/mcp-server",
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "invalid transport",
			def: ServerDef{
				Name:      "test-server",
				Transport: "grpc",
			},
			wantErr: true,
			errMsg:  "invalid transport",
		},
		{
			name: "stdio without command",
			def: ServerDef{
				Name:      "test-server",
				Transport: TransportStdio,
			},
			wantErr: true,
			errMsg:  "command is required for stdio transport",
		},
		{
			name: "sse without url",
			def: ServerDef{
				Name:      "test-server",
				Transport: TransportSSE,
			},
			wantErr: true,
			errMsg:  "url is required for sse transport",
		},
		{
			name: "empty transport",
			def: ServerDef{
				Name: "test-server",
			},
			wantErr: true,
			errMsg:  "transport is required",
		},
		{
			name: "valid streamable_http server",
			def: ServerDef{
				Name:      "streaming-server",
				Transport: TransportStreamableHTTP,
				URL:       "http://localhost:9090/mcp",
			},
			wantErr: false,
		},
		{
			name: "streamable_http without url",
			def: ServerDef{
				Name:      "test-server",
				Transport: TransportStreamableHTTP,
			},
			wantErr: true,
			errMsg:  "url is required for streamable_http transport",
		},
		{
			name: "stdio with all fields set",
			def: ServerDef{
				Name:      "full-server",
				Transport: TransportStdio,
				Command:   "npx",
				Args:      []string{"-y", "@modelcontextprotocol/server-filesystem"},
				Env:       map[string]string{"HOME": "/tmp"},
				Enabled:   true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.def.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, domain.ErrValidation) {
					t.Errorf("expected ErrValidation, got: %v", err)
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error to contain %q, got: %v", tt.errMsg, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
