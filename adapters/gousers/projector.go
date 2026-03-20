package gousers

import (
	"context"
	"strings"

	userstypes "github.com/goliatone/go-users/pkg/types"
	"github.com/goliatone/go-search/pkg/types"
)

type UserRecord struct {
	User    userstypes.AuthUser
	Profile *userstypes.UserProfile
	Scope   userstypes.ScopeFilter
}

type UserProjectorConfig struct {
	Index      string
	SourceType string
}

type UserProjector struct {
	cfg UserProjectorConfig
}

func NewUserProjector(cfg UserProjectorConfig) *UserProjector {
	return &UserProjector{cfg: cfg}
}

func (p *UserProjector) Project(_ context.Context, record UserRecord) ([]types.Document, error) {
	title := strings.TrimSpace(strings.TrimSpace(record.User.FirstName + " " + record.User.LastName))
	if title == "" {
		title = strings.TrimSpace(firstNonEmpty(record.ProfileName(), record.User.Username, record.User.Email))
	}
	doc := types.Document{
		ID:         record.User.ID.String(),
		Index:      p.cfg.Index,
		Type:       "user",
		SourceType: firstNonEmpty(p.cfg.SourceType, "user"),
		SourceID:   record.User.ID.String(),
		Title:      title,
		Summary:    strings.TrimSpace(record.User.Email),
		Body:       strings.TrimSpace(strings.Join(compact([]string{record.User.Email, record.User.Username, record.ProfileBio()}), " ")),
		Locale:     strings.TrimSpace(record.ProfileLocale()),
		Fields: map[string]any{
			"email":        record.User.Email,
			"username":     record.User.Username,
			"role":         record.User.Role,
			"status":       string(record.User.Status),
			"display_name": record.ProfileName(),
			"avatar_url":   record.ProfileAvatarURL(),
		},
		Facets: map[string][]string{
			"role":   compact([]string{record.User.Role}),
			"status": compact([]string{string(record.User.Status)}),
		},
		Booleans: map[string]bool{
			"active": record.User.Status == userstypes.LifecycleStateActive,
		},
		Scope: types.Scope{
			TenantID: record.Scope.TenantID.String(),
			OrgID:    record.Scope.OrgID.String(),
		},
		Visibility: types.Visibility{
			Public: false,
			Status: string(record.User.Status),
		},
		Metadata: cloneUserMetadata(record.User.Metadata),
	}
	return []types.Document{doc}, nil
}

func (r UserRecord) ProfileName() string {
	if r.Profile == nil {
		return ""
	}
	return strings.TrimSpace(r.Profile.DisplayName)
}

func (r UserRecord) ProfileAvatarURL() string {
	if r.Profile == nil {
		return ""
	}
	return strings.TrimSpace(r.Profile.AvatarURL)
}

func (r UserRecord) ProfileLocale() string {
	if r.Profile == nil {
		return ""
	}
	return strings.TrimSpace(r.Profile.Locale)
}

func (r UserRecord) ProfileBio() string {
	if r.Profile == nil {
		return ""
	}
	return strings.TrimSpace(r.Profile.Bio)
}

func cloneUserMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		out[key] = value
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func compact(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
