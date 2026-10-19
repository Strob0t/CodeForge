package otel

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
)

// http2Preface starts every plaintext HTTP/2 (gRPC) connection; a TLS
// connection starts with a handshake record (0x16) instead.
var http2Preface = []byte("PRI * HTTP/2.0")

// firstBytesOfExport runs InitTracer against a local TCP listener, exports one
// span and returns the first bytes the exporter sent on the connection.
func firstBytesOfExport(t *testing.T, insecure bool) []byte {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	received := make(chan []byte, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		buf := make([]byte, len(http2Preface))
		n, _ := io.ReadFull(conn, buf)
		received <- buf[:n]
	}()

	shutdown, err := InitTracer(OTELConfig{
		Enabled:     true,
		Endpoint:    ln.Addr().String(),
		ServiceName: "codeforge-test",
		Insecure:    insecure,
		SampleRate:  1.0,
	})
	if err != nil {
		t.Fatalf("InitTracer: %v", err)
	}
	_, span := otel.Tracer("insecure-test").Start(context.Background(), "export-me")
	span.End()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = shutdown(ctx) // the fake collector never answers; cancelled in cleanup
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	select {
	case got := <-received:
		return got
	case <-time.After(10 * time.Second):
		t.Fatal("the exporter never connected to the collector")
		return nil
	}
}

// TestInitTracer_Insecure: CODEFORGE_OTEL_INSECURE=true must reach a plaintext
// collector (the dev Jaeger); without it the exporter must use TLS (KI-36).
// Not parallel: InitTracer sets the global tracer provider.
func TestInitTracer_Insecure(t *testing.T) {
	tests := []struct {
		name      string
		insecure  bool
		plaintext bool
	}{
		{"insecure speaks plaintext HTTP/2", true, true},
		{"secure starts a TLS handshake", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstBytesOfExport(t, tt.insecure)
			isPlaintext := bytes.Equal(got, http2Preface)
			if isPlaintext != tt.plaintext {
				t.Fatalf("first bytes = %q, plaintext = %v, want plaintext = %v", got, isPlaintext, tt.plaintext)
			}
			if !tt.plaintext && (len(got) == 0 || got[0] != 0x16) {
				t.Fatalf("first bytes = %q, want a TLS handshake record (0x16)", got)
			}
		})
	}
}
