package types

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
)

type Scope struct {
	TenantID string            `json:"tenant_id"`
	OrgID    string            `json:"org_id"`
	Labels   map[string]string `json:"labels"`
}

func (s Scope) Clone() Scope {
	out := s
	if len(s.Labels) > 0 {
		out.Labels = make(map[string]string, len(s.Labels))
		maps.Copy(out.Labels, s.Labels)
	}
	return out
}

type ActorRef struct {
	UserID   string         `json:"user_id"`
	TenantID string         `json:"tenant_id"`
	OrgID    string         `json:"org_id"`
	Metadata map[string]any `json:"metadata"`
}

type Visibility struct {
	Public      bool     `json:"public"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	Status      string   `json:"status"`
}

func (v Visibility) Clone() Visibility {
	out := v
	out.Roles = append([]string(nil), v.Roles...)
	out.Permissions = append([]string(nil), v.Permissions...)
	return out
}

type Authorizer interface {
	Can(ctx context.Context, permission string, resource string) bool
}

type ScopeGuard interface {
	AllowSearch(ctx context.Context, actor ActorRef, req SearchRequest) bool
	AllowSuggest(ctx context.Context, actor ActorRef, req SuggestRequest) bool
	AllowDocument(ctx context.Context, actor ActorRef, doc Document) bool
}

// ProviderEnforcedSearchScopeGuard is an optional ScopeGuard contract for
// hosts that compile document authorization into provider search filters.
//
// SearchAuthorizationEnforcedByProvider must return true only when every
// document matched by the normalized request is guaranteed to satisfy the
// same authorization policy as AllowDocument. When it returns true, the query
// layer trusts the provider result set and preserves provider totals, facets,
// and evidence accuracy instead of applying bounded post-filtering.
//
// Guards that do not implement this interface remain fail closed.
type ProviderEnforcedSearchScopeGuard interface {
	ScopeGuard
	SearchAuthorizationEnforcedByProvider(ctx context.Context, actor ActorRef, req SearchRequest) bool
}

// DefaultScopeGuard is the fail-closed authorization policy used when a host
// does not provide a more specific guard. Unscoped documents with no explicit
// visibility policy retain legacy public behavior; any scope or visibility
// constraint requires a matching authenticated actor.
type DefaultScopeGuard struct{}

// RequiresCandidateExpansion reports that authorization must run across the
// bounded candidate window so unauthorized hits cannot skew pages or facets.
func (DefaultScopeGuard) RequiresCandidateExpansion() bool { return true }

func (DefaultScopeGuard) AllowSearch(_ context.Context, actor ActorRef, req SearchRequest) bool {
	return requestScopeAllowed(actor, req.Scope)
}

func (DefaultScopeGuard) AllowSuggest(_ context.Context, actor ActorRef, req SuggestRequest) bool {
	return requestScopeAllowed(actor, req.Scope)
}

func (DefaultScopeGuard) AllowDocument(_ context.Context, actor ActorRef, doc Document) bool {
	if !documentScopeAllowed(actor, doc.Scope) {
		return false
	}
	visibility := doc.Visibility
	if visibility.Public {
		return true
	}
	if matchesActorValues(actor.Metadata, "role", "roles", visibility.Roles) ||
		matchesActorValues(actor.Metadata, "permission", "permissions", visibility.Permissions) {
		return strings.TrimSpace(actor.UserID) != ""
	}
	// A completely unconstrained legacy document is public. A scoped document
	// with no visibility fields is private to authenticated members of its scope.
	if strings.TrimSpace(visibility.Status) == "" && len(visibility.Roles) == 0 && len(visibility.Permissions) == 0 {
		if scopeEmpty(doc.Scope) {
			return true
		}
		return strings.TrimSpace(actor.UserID) != ""
	}
	return false
}

func requestScopeAllowed(actor ActorRef, scope Scope) bool {
	if scopeEmpty(scope) {
		return true
	}
	if strings.TrimSpace(actor.UserID) == "" {
		return false
	}
	if tenant := strings.TrimSpace(scope.TenantID); tenant != "" && !strings.EqualFold(tenant, strings.TrimSpace(actor.TenantID)) {
		return false
	}
	if org := strings.TrimSpace(scope.OrgID); org != "" && !strings.EqualFold(org, strings.TrimSpace(actor.OrgID)) {
		return false
	}
	return labelsAllowed(actor.Metadata, scope.Labels)
}

func documentScopeAllowed(actor ActorRef, scope Scope) bool {
	if scopeEmpty(scope) {
		return true
	}
	return requestScopeAllowed(actor, scope)
}

func scopeEmpty(scope Scope) bool {
	return strings.TrimSpace(scope.TenantID) == "" && strings.TrimSpace(scope.OrgID) == "" && len(scope.Labels) == 0
}

func labelsAllowed(metadata map[string]any, required map[string]string) bool {
	if len(required) == 0 {
		return true
	}
	raw, ok := metadata["scope_labels"]
	if !ok {
		return false
	}
	labels := map[string]string{}
	switch value := raw.(type) {
	case map[string]string:
		maps.Copy(labels, value)
	case map[string]any:
		for key, item := range value {
			labels[key] = strings.TrimSpace(fmt.Sprint(item))
		}
	default:
		return false
	}
	for key, want := range required {
		if !strings.EqualFold(strings.TrimSpace(labels[key]), strings.TrimSpace(want)) {
			return false
		}
	}
	return true
}

func matchesActorValues(metadata map[string]any, singular, plural string, allowed []string) bool {
	if len(allowed) == 0 || len(metadata) == 0 {
		return false
	}
	values := []string{}
	for _, key := range []string{singular, plural} {
		switch value := metadata[key].(type) {
		case string:
			values = append(values, value)
		case []string:
			values = append(values, value...)
		case []any:
			for _, item := range value {
				values = append(values, fmt.Sprint(item))
			}
		}
	}
	for _, candidate := range values {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if slices.ContainsFunc(allowed, func(value string) bool {
			return candidate != "" && candidate == strings.ToLower(strings.TrimSpace(value))
		}) {
			return true
		}
	}
	return false
}

type CapabilityGate interface {
	Enabled(ctx context.Context, feature string) bool
}

type Logger interface {
	Debug(msg string, metadata map[string]any)
	Info(msg string, metadata map[string]any)
	Warn(msg string, metadata map[string]any)
	Error(msg string, metadata map[string]any)
}

type MetricsHook interface {
	Observe(ctx context.Context, metric string, value float64, labels map[string]string)
	Count(ctx context.Context, metric string, delta int64, labels map[string]string)
}

type ActivityEvent struct {
	Channel    string         `json:"channel"`
	Verb       string         `json:"verb"`
	ObjectType string         `json:"object_type"`
	ObjectID   string         `json:"object_id"`
	RecordID   string         `json:"record_id,omitempty"`
	ActorID    string         `json:"actor_id"`
	TenantID   string         `json:"tenant_id"`
	OrgID      string         `json:"org_id,omitempty"`
	OccurredAt int64          `json:"occurred_at"`
	Metadata   map[string]any `json:"metadata"`
}

type ActivityHook interface {
	Notify(ctx context.Context, event ActivityEvent)
}

type ProgressUpdate struct {
	Index     string         `json:"index"`
	Completed int            `json:"completed"`
	Total     int            `json:"total"`
	Message   string         `json:"message"`
	Metadata  map[string]any `json:"metadata"`
}

type ProgressReporter interface {
	Report(ctx context.Context, update ProgressUpdate)
}
