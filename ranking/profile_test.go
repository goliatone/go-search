package ranking

import (
	"github.com/goliatone/go-search/pkg/types"
	"reflect"
	"testing"
)

func TestLegacyQueryFieldsAndRegistry(t *testing.T) {
	fields := LegacyQueryFields(types.IndexDefinition{DefaultQueryFields: []string{"title", "body"}})
	if !reflect.DeepEqual(fields, []types.QueryField{{Field: "title", Weight: 1}, {Field: "body", Weight: 1}}) {
		t.Fatalf("fields=%#v", fields)
	}
	r := NewRegistry("public-v1")
	if err := r.RegisterImplementation("rrf"); err != nil {
		t.Fatal(err)
	}
	p := RankingProfile{Name: "public-v1", Version: "1", Indexes: map[string]IndexProfile{"media": {QueryFields: []types.QueryField{{Field: "title", Weight: 10}}}}, Candidates: CandidateConfig{Multiplier: 5, MaxPerIndex: 250, MaxRefillRounds: 2}, Fusion: ImplementationRef{ID: "rrf"}}
	if err := r.Register(p); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(p); err == nil {
		t.Fatal("expected duplicate error")
	}
	if err := r.Seal(); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Resolve("public-v1"); !ok {
		t.Fatal("profile missing")
	}
	got, _ := r.Resolve("public-v1")
	cfg := got.Indexes["media"]
	cfg.QueryFields[0].Weight = 99
	got.Indexes["media"] = cfg
	again, _ := r.Resolve("public-v1")
	if again.Indexes["media"].QueryFields[0].Weight != 10 {
		t.Fatal("resolved profile mutated registry")
	}
}

func TestRegistryRejectsUnknownImplementation(t *testing.T) {
	r := NewRegistry("")
	p := RankingProfile{Name: "p", Version: "1", Indexes: map[string]IndexProfile{}, Candidates: CandidateConfig{Multiplier: 1, MaxPerIndex: 1, MaxRefillRounds: 1}, Fusion: ImplementationRef{ID: "missing"}}
	if err := r.Register(p); err == nil {
		t.Fatal("expected unknown implementation error")
	}
}

func TestProfileValidationRejectsInvalidFields(t *testing.T) {
	p := RankingProfile{Name: "p", Version: "1", Indexes: map[string]IndexProfile{"media": {QueryFields: []types.QueryField{{Field: "title", Weight: 1}, {Field: "title", Weight: 2}}}}, Candidates: CandidateConfig{Multiplier: 1, MaxPerIndex: 1, MaxRefillRounds: 1}}
	if err := p.Validate(); err == nil {
		t.Fatal("expected duplicate field error")
	}
}
