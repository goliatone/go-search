package release

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	SearchV1TeamBackend  = "be"
	SearchV1TeamFrontend = "fe"
	SearchV1TeamQA       = "qa"
	SearchV1TeamOps      = "ops"
)

var searchV1RequiredTeams = []string{
	SearchV1TeamBackend,
	SearchV1TeamFrontend,
	SearchV1TeamQA,
	SearchV1TeamOps,
}

type SearchV1Signoff struct {
	Approved   bool   `json:"approved"`
	Approver   string `json:"approver"`
	ApprovedAt string `json:"approved_at"`
	Ticket     string `json:"ticket"`
}

type SearchV1ProviderMatrix struct {
	Memory    bool `json:"memory"`
	Typesense bool `json:"typesense"`
	Postgres  bool `json:"postgres"`
}

type SearchV1Evidence struct {
	GoI18NRef             string `json:"go_i18n_ref"`
	GoSearchRef           string `json:"go_search_ref"`
	ExampleRef            string `json:"example_ref"`
	GoAdminRef            string `json:"go_admin_ref"`
	GoCMSRef              string `json:"go_cms_ref"`
	GoUsersRef            string `json:"go_users_ref"`
	RunbookRef            string `json:"runbook_ref"`
	VersionMatrixRef      string `json:"version_matrix_ref"`
	CompatibilityNotesRef string `json:"compatibility_notes_ref"`
	VersionSkewRef        string `json:"version_skew_ref"`
	FollowupsRef          string `json:"followups_ref"`
}

type SearchV1Rollback struct {
	ProviderEnv     string   `json:"provider_env"`
	CacheEnv        string   `json:"cache_env"`
	EditorialEnv    string   `json:"editorial_env"`
	FeatureFlagsOff []string `json:"feature_flags_off"`
	Notes           string   `json:"notes"`
}

type SearchV1ReleaseChecklist struct {
	ReleaseID         string                     `json:"release_id"`
	Phase             string                     `json:"phase"`
	RequiredProviders SearchV1ProviderMatrix     `json:"required_providers"`
	Signoffs          map[string]SearchV1Signoff `json:"signoffs"`
	Evidence          SearchV1Evidence           `json:"evidence"`
	Rollback          SearchV1Rollback           `json:"rollback"`
}

func (c SearchV1ReleaseChecklist) Validate() []string {
	issues := make([]string, 0)
	if strings.TrimSpace(c.ReleaseID) == "" {
		issues = append(issues, "release_id is required")
	}
	if strings.TrimSpace(c.Phase) != "10" {
		issues = append(issues, "phase must be 10")
	}
	if !c.RequiredProviders.Memory || !c.RequiredProviders.Typesense || !c.RequiredProviders.Postgres {
		issues = append(issues, "required_providers must include memory, typesense, and postgres")
	}
	if c.Signoffs == nil {
		issues = append(issues, "signoffs are required")
	} else {
		for _, team := range searchV1RequiredTeams {
			signoff, ok := c.Signoffs[team]
			if !ok {
				issues = append(issues, fmt.Sprintf("missing %s signoff", team))
				continue
			}
			if !signoff.Approved {
				issues = append(issues, fmt.Sprintf("%s signoff is not approved", team))
			}
			if strings.TrimSpace(signoff.Approver) == "" {
				issues = append(issues, fmt.Sprintf("%s approver is required", team))
			}
			if strings.TrimSpace(signoff.ApprovedAt) == "" {
				issues = append(issues, fmt.Sprintf("%s approved_at is required", team))
			}
			if strings.TrimSpace(signoff.Ticket) == "" {
				issues = append(issues, fmt.Sprintf("%s ticket is required", team))
			}
		}
	}
	for label, ref := range map[string]string{
		"go_i18n_ref":             c.Evidence.GoI18NRef,
		"go_search_ref":           c.Evidence.GoSearchRef,
		"example_ref":             c.Evidence.ExampleRef,
		"go_admin_ref":            c.Evidence.GoAdminRef,
		"go_cms_ref":              c.Evidence.GoCMSRef,
		"go_users_ref":            c.Evidence.GoUsersRef,
		"runbook_ref":             c.Evidence.RunbookRef,
		"version_matrix_ref":      c.Evidence.VersionMatrixRef,
		"compatibility_notes_ref": c.Evidence.CompatibilityNotesRef,
		"version_skew_ref":        c.Evidence.VersionSkewRef,
		"followups_ref":           c.Evidence.FollowupsRef,
	} {
		if strings.TrimSpace(ref) == "" {
			issues = append(issues, fmt.Sprintf("%s is required", label))
		}
	}
	if strings.TrimSpace(c.Rollback.ProviderEnv) != "APP_SEARCH_DEMO__PROVIDER" {
		issues = append(issues, "rollback.provider_env must be APP_SEARCH_DEMO__PROVIDER")
	}
	if strings.TrimSpace(c.Rollback.CacheEnv) != "APP_SEARCH_DEMO__CACHE_ENABLED" {
		issues = append(issues, "rollback.cache_env must be APP_SEARCH_DEMO__CACHE_ENABLED")
	}
	if strings.TrimSpace(c.Rollback.EditorialEnv) != "APP_SEARCH_DEMO__EDITORIAL_ENABLED" {
		issues = append(issues, "rollback.editorial_env must be APP_SEARCH_DEMO__EDITORIAL_ENABLED")
	}
	if !containsAll(c.Rollback.FeatureFlagsOff, "search", "cms", "users") {
		issues = append(issues, "rollback.feature_flags_off must include search, cms, and users")
	}
	if strings.TrimSpace(c.Rollback.Notes) == "" {
		issues = append(issues, "rollback.notes are required")
	}
	return issues
}

func SearchV1RequiredSignoffTeams() []string {
	return slices.Clone(searchV1RequiredTeams)
}

func LoadSearchV1ReleaseChecklist(path string) (SearchV1ReleaseChecklist, error) {
	raw, err := readFileFromWorkingDir(path)
	if err != nil {
		return SearchV1ReleaseChecklist{}, err
	}
	var checklist SearchV1ReleaseChecklist
	if err := json.Unmarshal(raw, &checklist); err != nil {
		return SearchV1ReleaseChecklist{}, err
	}
	return checklist, nil
}

func readFileFromWorkingDir(path string) ([]byte, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}

	requested := filepath.Clean(path)
	if !filepath.IsAbs(requested) {
		requested = filepath.Join(wd, requested)
	}

	rel, err := filepath.Rel(wd, requested)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("path must be under working directory")
	}

	root, err := os.OpenRoot(wd)
	if err != nil {
		return nil, fmt.Errorf("open working directory root: %w", err)
	}
	defer func() {
		_ = root.Close()
	}()

	file, err := root.Open(rel)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()

	return io.ReadAll(file)
}

func ValidateSearchV1ReleaseChecklistFile(path string) (SearchV1ReleaseChecklist, []string, error) {
	checklist, err := LoadSearchV1ReleaseChecklist(path)
	if err != nil {
		return SearchV1ReleaseChecklist{}, nil, err
	}
	return checklist, checklist.Validate(), nil
}

func containsAll(values []string, required ...string) bool {
	if len(required) == 0 {
		return true
	}
	lookup := map[string]struct{}{}
	for _, value := range values {
		lookup[strings.TrimSpace(value)] = struct{}{}
	}
	for _, key := range required {
		if _, ok := lookup[strings.TrimSpace(key)]; !ok {
			return false
		}
	}
	return true
}
