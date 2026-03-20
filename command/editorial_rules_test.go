package command

import (
	"context"
	"testing"

	"github.com/goliatone/go-search/pkg/types"
)

type memoryEditorialStore struct {
	rules map[string]types.EditorialRankRule
}

func newMemoryEditorialStore() *memoryEditorialStore {
	return &memoryEditorialStore{rules: map[string]types.EditorialRankRule{}}
}

func (s *memoryEditorialStore) ListApplicable(_ context.Context, req types.SearchRequest) ([]types.EditorialRankRule, error) {
	out := []types.EditorialRankRule{}
	for _, rule := range s.rules {
		if len(rule.Scope.Indexes) == 0 || len(req.Indexes) == 0 || rule.Scope.Indexes[0] == req.Indexes[0] {
			if rule.Enabled {
				out = append(out, rule)
			}
		}
	}
	return out, nil
}

func (s *memoryEditorialStore) Upsert(_ context.Context, rule types.EditorialRankRule) error {
	s.rules[rule.ID] = rule
	return nil
}

func (s *memoryEditorialStore) Delete(_ context.Context, id string) error {
	delete(s.rules, id)
	return nil
}

func (s *memoryEditorialStore) List(_ context.Context, _ types.EditorialRuleListRequest) ([]types.EditorialRankRule, error) {
	out := make([]types.EditorialRankRule, 0, len(s.rules))
	for _, rule := range s.rules {
		out = append(out, rule)
	}
	return out, nil
}

func (s *memoryEditorialStore) SetEnabled(_ context.Context, id string, enabled bool) error {
	rule := s.rules[id]
	rule.Enabled = enabled
	s.rules[id] = rule
	return nil
}

func TestUpsertEditorialRuleRejectsSegmentTargetOnlyRules(t *testing.T) {
	cmd, err := NewUpsertEditorialRule(UpsertEditorialRuleConfig{Store: newMemoryEditorialStore()})
	if err != nil {
		t.Fatalf("new command: %v", err)
	}
	err = cmd.Execute(context.Background(), types.UpsertEditorialRuleInput{
		Rule: types.EditorialRankRule{
			ID:       "rule-1",
			TargetID: "segment-1",
			Action:   types.EditorialActionPin,
			Enabled:  true,
		},
	})
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestEditorialRuleCommandsRoundTrip(t *testing.T) {
	store := newMemoryEditorialStore()
	upsert, err := NewUpsertEditorialRule(UpsertEditorialRuleConfig{Store: store})
	if err != nil {
		t.Fatalf("new upsert command: %v", err)
	}
	toggle, err := NewSetEditorialRuleEnabled(SetEditorialRuleEnabledConfig{Store: store})
	if err != nil {
		t.Fatalf("new toggle command: %v", err)
	}
	remove, err := NewDeleteEditorialRule(DeleteEditorialRuleConfig{Store: store})
	if err != nil {
		t.Fatalf("new delete command: %v", err)
	}
	rule := types.EditorialRankRule{
		ID:             "rule-1",
		ParentTargetID: "video-1",
		Action:         types.EditorialActionPin,
		Enabled:        true,
	}
	if err := upsert.Execute(context.Background(), types.UpsertEditorialRuleInput{Rule: rule}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if len(store.rules) != 1 {
		t.Fatalf("rules = %#v", store.rules)
	}
	if err := toggle.Execute(context.Background(), types.SetEditorialRuleEnabledInput{ID: "rule-1", Enabled: false}); err != nil {
		t.Fatalf("toggle: %v", err)
	}
	if store.rules["rule-1"].Enabled {
		t.Fatalf("expected rule disabled")
	}
	if err := remove.Execute(context.Background(), types.DeleteEditorialRuleInput{ID: "rule-1"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(store.rules) != 0 {
		t.Fatalf("rules = %#v", store.rules)
	}
}
