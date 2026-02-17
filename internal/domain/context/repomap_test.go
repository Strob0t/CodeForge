package context

import "testing"

func TestRepoMapValidateValid(t *testing.T) {
	m := RepoMap{
		ProjectID: "proj-1",
		MapText:   "src/main.go\n  func main()\n",
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("expected valid, got error: %v", err)
	}
}

func TestRepoMapValidateMissingProjectID(t *testing.T) {
	m := RepoMap{
		MapText: "src/main.go\n  func main()\n",
	}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
	if err.Error() != "project_id is required" {
		t.Fatalf("unexpected error message: %s", err.Error())
	}
}

func TestRepoMapValidateMissingMapText(t *testing.T) {
	m := RepoMap{
		ProjectID: "proj-1",
	}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for missing map_text")
	}
	if err.Error() != "map_text is required" {
		t.Fatalf("unexpected error message: %s", err.Error())
	}
}
