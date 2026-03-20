package types

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCapabilitySetJSONUsesSnakeCase(t *testing.T) {
	payload, err := json.Marshal(CapabilitySet{
		PrefixSearch:         true,
		SupportedSearchModes: []SearchMode{SearchModeLexical},
	})
	if err != nil {
		t.Fatalf("marshal capability set: %v", err)
	}
	body := string(payload)
	if !strings.Contains(body, `"prefix_search":true`) {
		t.Fatalf("expected snake_case field in %s", body)
	}
	if !strings.Contains(body, `"supported_search_modes":["lexical"]`) {
		t.Fatalf("expected supported_search_modes in %s", body)
	}
	if strings.Contains(body, `"PrefixSearch"`) || strings.Contains(body, `"SupportedSearchModes"`) {
		t.Fatalf("expected no PascalCase fields in %s", body)
	}
}

func TestHealthStatusJSONUsesSnakeCase(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	payload, err := json.Marshal(HealthStatus{
		Provider:  "memory",
		Healthy:   true,
		CheckedAt: now,
		Indexes: []IndexHealth{
			{Name: "media_transcripts", Ready: true, Documents: 3},
		},
	})
	if err != nil {
		t.Fatalf("marshal health status: %v", err)
	}
	body := string(payload)
	for _, key := range []string{`"provider"`, `"healthy"`, `"checked_at"`, `"indexes"`, `"documents"`} {
		if !strings.Contains(body, key) {
			t.Fatalf("expected %s in %s", key, body)
		}
	}
	for _, key := range []string{`"Provider"`, `"CheckedAt"`, `"Documents"`} {
		if strings.Contains(body, key) {
			t.Fatalf("unexpected PascalCase key %s in %s", key, body)
		}
	}
}

func TestSearchRequestJSONUsesSnakeCase(t *testing.T) {
	payload, err := json.Marshal(SearchRequest{
		Indexes:       []string{"media_transcripts"},
		Query:         "transcript",
		PerPage:       10,
		GroupBy:       "parent_id",
		IncludeFields: []string{"title"},
	})
	if err != nil {
		t.Fatalf("marshal search request: %v", err)
	}
	body := string(payload)
	for _, key := range []string{`"indexes"`, `"query"`, `"per_page"`, `"group_by"`, `"include_fields"`} {
		if !strings.Contains(body, key) {
			t.Fatalf("expected %s in %s", key, body)
		}
	}
	for _, key := range []string{`"PerPage"`, `"GroupBy"`, `"IncludeFields"`} {
		if strings.Contains(body, key) {
			t.Fatalf("unexpected PascalCase key %s in %s", key, body)
		}
	}
}
