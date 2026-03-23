package gousers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	searchtypes "github.com/goliatone/go-search/pkg/types"
	userstypes "github.com/goliatone/go-users/pkg/types"
	"github.com/google/uuid"
)

type ResolveScopeFunc func(context.Context, userstypes.AuthUser, *userstypes.UserProfile) (userstypes.ScopeFilter, error)

type UserRecordLoader interface {
	Get(ctx context.Context, id string) (UserRecord, error)
	List(ctx context.Context, limit int, cursor string) ([]UserRecord, string, error)
}

type Source struct {
	loader UserRecordLoader
}

func NewSource(loader UserRecordLoader) *Source {
	return &Source{loader: loader}
}

func (s *Source) Get(ctx context.Context, id string) (UserRecord, error) {
	if s == nil || s.loader == nil {
		return UserRecord{}, fmt.Errorf("user source unavailable")
	}
	return s.loader.Get(ctx, id)
}

func (s *Source) List(ctx context.Context, limit int, cursor string) ([]UserRecord, string, error) {
	if s == nil || s.loader == nil {
		return nil, "", fmt.Errorf("user source unavailable")
	}
	return s.loader.List(ctx, limit, cursor)
}

type RepositoryLoaderConfig struct {
	Users        userstypes.AuthRepository
	Inventory    userstypes.UserInventoryRepository
	Profiles     userstypes.ProfileRepository
	Actor        userstypes.ActorRef
	Scope        userstypes.ScopeFilter
	Statuses     []userstypes.LifecycleState
	ResolveScope ResolveScopeFunc
	Logger       searchtypes.Logger
}

type RepositoryLoader struct {
	users        userstypes.AuthRepository
	inventory    userstypes.UserInventoryRepository
	profiles     userstypes.ProfileRepository
	actor        userstypes.ActorRef
	scope        userstypes.ScopeFilter
	statuses     []userstypes.LifecycleState
	resolveScope ResolveScopeFunc
	logger       searchtypes.Logger
}

func NewRepositoryLoader(cfg RepositoryLoaderConfig) (*RepositoryLoader, error) {
	if cfg.Users == nil {
		return nil, fmt.Errorf("user auth repository is required")
	}
	if cfg.Inventory == nil {
		return nil, fmt.Errorf("user inventory repository is required")
	}
	if cfg.ResolveScope == nil {
		return nil, fmt.Errorf("user scope resolver is required")
	}
	return &RepositoryLoader{
		users:        cfg.Users,
		inventory:    cfg.Inventory,
		profiles:     cfg.Profiles,
		actor:        cfg.Actor,
		scope:        cfg.Scope.Clone(),
		statuses:     append([]userstypes.LifecycleState(nil), cfg.Statuses...),
		resolveScope: cfg.ResolveScope,
		logger:       cfg.Logger,
	}, nil
}

func (l *RepositoryLoader) Get(ctx context.Context, id string) (UserRecord, error) {
	if l == nil || l.users == nil {
		return UserRecord{}, fmt.Errorf("user loader unavailable")
	}
	uid, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return UserRecord{}, err
	}
	user, err := l.users.GetByID(ctx, uid)
	if err != nil {
		return UserRecord{}, err
	}
	if user == nil {
		return UserRecord{}, fmt.Errorf("user %q not found", id)
	}
	return l.buildRecord(ctx, *user)
}

func (l *RepositoryLoader) List(ctx context.Context, limit int, cursor string) ([]UserRecord, string, error) {
	if l == nil || l.inventory == nil {
		return nil, "", fmt.Errorf("user loader unavailable")
	}
	offset, err := parseOffset(cursor)
	if err != nil {
		return nil, "", err
	}
	page, err := l.inventory.ListUsers(ctx, userstypes.UserInventoryFilter{
		Actor:    l.actor,
		Scope:    l.scope.Clone(),
		Statuses: append([]userstypes.LifecycleState(nil), l.statuses...),
		Pagination: userstypes.Pagination{
			Limit:  limit,
			Offset: offset,
		},
	})
	if err != nil {
		return nil, "", err
	}
	records := make([]UserRecord, 0, len(page.Users))
	for _, user := range page.Users {
		record, err := l.buildRecord(ctx, user)
		if err != nil {
			if l.logger != nil {
				l.logger.Warn("search.adapter.gousers.record_skipped", map[string]any{
					"record_id": user.ID.String(),
					"message":   err.Error(),
				})
			}
			continue
		}
		records = append(records, record)
	}
	next := ""
	if page.HasMore {
		next = strconv.Itoa(page.NextOffset)
	}
	return records, next, nil
}

func (l *RepositoryLoader) buildRecord(ctx context.Context, user userstypes.AuthUser) (UserRecord, error) {
	scope, err := l.resolveScope(ctx, user, nil)
	if err != nil {
		return UserRecord{}, err
	}
	if scopeEmpty(scope) {
		return UserRecord{}, fmt.Errorf("scope resolver returned empty scope")
	}
	var profile *userstypes.UserProfile
	if l.profiles != nil {
		profile, err = l.profiles.GetProfile(ctx, user.ID, scope)
		if err != nil {
			return UserRecord{}, err
		}
		if resolved, err := l.resolveScope(ctx, user, profile); err != nil {
			return UserRecord{}, err
		} else if !scopeEmpty(resolved) {
			scope = resolved
		}
	}
	if scopeEmpty(scope) {
		return UserRecord{}, fmt.Errorf("resolved scope is empty")
	}
	return UserRecord{
		User:    user,
		Profile: profile,
		Scope:   scope,
	}, nil
}

func parseOffset(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid cursor %q", raw)
	}
	if value < 0 {
		return 0, fmt.Errorf("invalid negative cursor %q", raw)
	}
	return value, nil
}

func scopeEmpty(scope userstypes.ScopeFilter) bool {
	return scope.TenantID == uuid.Nil && scope.OrgID == uuid.Nil && len(scope.Labels) == 0
}
