package searchdemo

import (
	"context"
	"testing"
)

func TestRuntimeBootstrapsSeededSearch(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	status, err := runtime.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Documents != 3 {
		t.Fatalf("expected 3 seeded documents, got %d", status.Documents)
	}

	result, err := runtime.Search(context.Background(), SearchRequest{
		Query:  "transcript",
		Locale: "en",
		Group:  true,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total == 0 {
		t.Fatalf("expected seeded results, got none")
	}

	suggest, err := runtime.Suggest(context.Background(), SuggestRequest{
		Query: "search",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if len(suggest.Items) == 0 {
		t.Fatalf("expected suggestions, got none")
	}
}

func TestRuntimeBindsAcceptLanguageAndFallsBackThroughLocalePolicy(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	bound := runtime.BindSearchRequest(SearchRequest{
		Query:          "transcript",
		AcceptLanguage: "fr-CA,fr;q=0.9,en;q=0.8",
	})
	if bound.Locale != "en" {
		t.Fatalf("bound locale = %q", bound.Locale)
	}
	if bound.LocaleSource != "accept_language" {
		t.Fatalf("locale source = %q", bound.LocaleSource)
	}
	if !bound.LocaleSupported {
		t.Fatalf("expected locale to be supported")
	}

	result, err := runtime.Search(context.Background(), SearchRequest{
		Query:  "transcript",
		Locale: "es-MX",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total == 0 {
		t.Fatalf("expected fallback results for es-MX request")
	}
	if len(result.Hits) == 0 {
		t.Fatalf("expected hits, got none")
	}
	if result.Hits[0].Retrieval == nil || result.Hits[0].Retrieval.Metadata["locale_origin"] != "fallback" {
		t.Fatalf("expected fallback locale annotation, got %+v", result.Hits[0].Retrieval)
	}
}
