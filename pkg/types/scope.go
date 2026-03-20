package types

import "context"

type Scope struct {
	TenantID string
	OrgID    string
	Labels   map[string]string
}

func (s Scope) Clone() Scope {
	out := s
	if len(s.Labels) > 0 {
		out.Labels = make(map[string]string, len(s.Labels))
		for k, v := range s.Labels {
			out.Labels[k] = v
		}
	}
	return out
}

type ActorRef struct {
	UserID   string
	TenantID string
	OrgID    string
	Metadata map[string]any
}

type Visibility struct {
	Public      bool
	Roles       []string
	Permissions []string
	Status      string
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
	Channel    string
	Verb       string
	ObjectType string
	ObjectID   string
	ActorID    string
	TenantID   string
	OccurredAt int64
	Metadata   map[string]any
}

type ActivityHook interface {
	Notify(ctx context.Context, event ActivityEvent)
}

type ProgressUpdate struct {
	Index     string
	Completed int
	Total     int
	Message   string
	Metadata  map[string]any
}

type ProgressReporter interface {
	Report(ctx context.Context, update ProgressUpdate)
}

type LocalePolicy interface {
	Normalize(locale string) string
	NormalizeMany(locales []string) []string
}
