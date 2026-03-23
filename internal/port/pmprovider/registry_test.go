package pmprovider_test

import (
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/port/pmprovider"
)

func TestRegister_DuplicatePanics(t *testing.T) {
	name := "test-dup-pm-" + t.Name()

	// First registration must succeed.
	pmprovider.Register(name, func(_ map[string]string) (pmprovider.Provider, error) {
		return nil, nil
	})

	// Second registration with the same name must panic.
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on duplicate registration, got none")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic, got %T: %v", r, r)
		}
		if !strings.Contains(msg, "duplicate") {
			t.Errorf("panic message = %q, want it to contain %q", msg, "duplicate")
		}
	}()

	pmprovider.Register(name, func(_ map[string]string) (pmprovider.Provider, error) {
		return nil, nil
	})
}

func TestNew_UnknownProvider(t *testing.T) {
	_, err := pmprovider.New("nonexistent-pm-provider-"+t.Name(), nil)
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
	if !strings.Contains(err.Error(), "unknown provider") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "unknown provider")
	}
}

func TestAvailable_ReturnsSorted(t *testing.T) {
	// Register two providers with names that sort in a known order.
	nameA := "aaa-pm-" + t.Name()
	nameZ := "zzz-pm-" + t.Name()

	pmprovider.Register(nameZ, func(_ map[string]string) (pmprovider.Provider, error) {
		return nil, nil
	})
	pmprovider.Register(nameA, func(_ map[string]string) (pmprovider.Provider, error) {
		return nil, nil
	})

	names := pmprovider.Available()

	// Find the indices of our two names.
	idxA, idxZ := -1, -1
	for i, n := range names {
		if n == nameA {
			idxA = i
		}
		if n == nameZ {
			idxZ = i
		}
	}

	if idxA < 0 {
		t.Fatalf("expected %q in Available(), not found", nameA)
	}
	if idxZ < 0 {
		t.Fatalf("expected %q in Available(), not found", nameZ)
	}
	if idxA >= idxZ {
		t.Errorf("expected %q (idx %d) before %q (idx %d)", nameA, idxA, nameZ, idxZ)
	}
}
