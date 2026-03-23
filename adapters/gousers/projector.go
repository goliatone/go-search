package gousers

import (
	"context"
	"fmt"
	"strings"

	"github.com/goliatone/go-search/pkg/types"
	userstypes "github.com/goliatone/go-users/pkg/types"
	"github.com/google/uuid"
)

type UserRecord struct {
	User    userstypes.AuthUser
	Profile *userstypes.UserProfile
	Scope   userstypes.ScopeFilter
}

type VisibilityResolver func(context.Context, userstypes.AuthUser, *userstypes.UserProfile, userstypes.ScopeFilter) types.Visibility

type UserProjectorConfig struct {
	Index             string
	SourceType        string
	ResolveVisibility VisibilityResolver
}

type UserProjector struct {
	cfg UserProjectorConfig
}

func NewUserProjector(cfg UserProjectorConfig) *UserProjector {
	return &UserProjector{cfg: cfg}
}

func (p *UserProjector) Project(ctx context.Context, record UserRecord) ([]types.Document, error) {
	if scopeEmpty(record.Scope) {
		return nil, fmt.Errorf("user record %s is missing scope", record.User.ID.String())
	}
	title := strings.TrimSpace(strings.TrimSpace(record.User.FirstName + " " + record.User.LastName))
	if title == "" {
		title = strings.TrimSpace(firstNonEmpty(record.ProfileName(), record.User.Username, record.User.Email))
	}
	locale := strings.TrimSpace(record.ProfileLocale())
	if locale != "" {
		locale = userstypes.NormalizeLocale(locale)
	}
	visibility := p.resolveVisibility(ctx, record)
	doc := types.Document{
		ID:         record.User.ID.String(),
		Index:      p.cfg.Index,
		Type:       "user",
		SourceType: firstNonEmpty(p.cfg.SourceType, "user"),
		SourceID:   record.User.ID.String(),
		Title:      title,
		Summary:    strings.TrimSpace(record.User.Email),
		Body:       strings.TrimSpace(strings.Join(compact([]string{record.User.Email, record.User.Username, record.ProfileName(), record.ProfileBio(), contactBody(record.ProfileContact())}), " ")),
		Locale:     locale,
		Fields: map[string]any{
			"email":        record.User.Email,
			"username":     record.User.Username,
			"user_id":      record.User.ID.String(),
			"role":         record.User.Role,
			"status":       string(record.User.Status),
			"display_name": record.ProfileName(),
			"avatar_url":   record.ProfileAvatarURL(),
			"tenant_id":    scopeUUIDString(record.Scope.TenantID),
			"org_id":       scopeUUIDString(record.Scope.OrgID),
			"scope_labels": scopeLabelStrings(record.Scope.Labels),
		},
		Facets: map[string][]string{
			"entity_type": {"user"},
			"role":        compact([]string{record.User.Role}),
			"status":      compact([]string{string(record.User.Status)}),
		},
		Booleans: map[string]bool{
			"active": record.User.Status == userstypes.LifecycleStateActive,
		},
		Scope: types.Scope{
			TenantID: scopeUUIDString(record.Scope.TenantID),
			OrgID:    scopeUUIDString(record.Scope.OrgID),
			Labels:   scopeLabelStrings(record.Scope.Labels),
		},
		Visibility: visibility,
		Metadata:   cloneUserMetadata(record.User.Metadata),
	}
	return []types.Document{doc}, nil
}

func (p *UserProjector) resolveVisibility(ctx context.Context, record UserRecord) types.Visibility {
	if p != nil && p.cfg.ResolveVisibility != nil {
		return p.cfg.ResolveVisibility(ctx, record.User, record.Profile, record.Scope)
	}
	return types.Visibility{
		Public: false,
		Status: string(record.User.Status),
	}
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

func (r UserRecord) ProfileContact() map[string]any {
	if r.Profile == nil {
		return nil
	}
	return r.Profile.Contact
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

func contactBody(contact map[string]any) string {
	if len(contact) == 0 {
		return ""
	}
	values := make([]string, 0, len(contact))
	for _, value := range contact {
		values = append(values, strings.TrimSpace(fmt.Sprint(value)))
	}
	return strings.Join(compact(values), " ")
}

func scopeUUIDString(value uuid.UUID) string {
	if value == uuid.Nil {
		return ""
	}
	return strings.TrimSpace(value.String())
}

func scopeLabelStrings(labels map[string]uuid.UUID) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		if value == uuid.Nil {
			continue
		}
		out[key] = value.String()
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
