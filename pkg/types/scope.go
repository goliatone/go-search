package types

import "maps"

import "context"

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
