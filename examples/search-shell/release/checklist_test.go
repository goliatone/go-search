package release

import (
	"path/filepath"
	"testing"
)

func TestSearchV1ReleaseChecklistTemplateRemainsPending(t *testing.T) {
	path := filepath.Join(".", "search_v1_release_checklist.json")
	checklist, issues, err := ValidateSearchV1ReleaseChecklistFile(path)
	if err != nil {
		t.Fatalf("validate checklist: %v", err)
	}
	if checklist.Phase != "10" {
		t.Fatalf("phase = %q", checklist.Phase)
	}
	if len(issues) == 0 {
		t.Fatalf("expected pending release checklist to fail validation")
	}
}

func TestApprovedSearchV1ReleaseChecklistSampleValidates(t *testing.T) {
	path := filepath.Join(".", "testdata", "search_v1_release_checklist_approved_sample.json")
	_, issues, err := ValidateSearchV1ReleaseChecklistFile(path)
	if err != nil {
		t.Fatalf("validate approved sample: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("approved sample issues = %v", issues)
	}
}
