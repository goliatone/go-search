package gousers

import (
	"context"
	"testing"

	searchtypes "github.com/goliatone/go-search/pkg/types"
	userstypes "github.com/goliatone/go-users/pkg/types"
	"github.com/google/uuid"
)

type noopLogger struct{}

func (noopLogger) Debug(string, map[string]any) {}
func (noopLogger) Info(string, map[string]any)  {}
func (noopLogger) Warn(string, map[string]any)  {}
func (noopLogger) Error(string, map[string]any) {}

type fakeAuthRepo struct {
	users map[uuid.UUID]userstypes.AuthUser
}

func (r *fakeAuthRepo) GetByID(_ context.Context, id uuid.UUID) (*userstypes.AuthUser, error) {
	user, ok := r.users[id]
	if !ok {
		return nil, nil
	}
	copy := user
	return &copy, nil
}

func (r *fakeAuthRepo) GetByIdentifier(context.Context, string) (*userstypes.AuthUser, error) {
	return nil, nil
}
func (r *fakeAuthRepo) Create(context.Context, *userstypes.AuthUser) (*userstypes.AuthUser, error) {
	return nil, nil
}
func (r *fakeAuthRepo) Update(context.Context, *userstypes.AuthUser) (*userstypes.AuthUser, error) {
	return nil, nil
}
func (r *fakeAuthRepo) UpdateStatus(context.Context, userstypes.ActorRef, uuid.UUID, userstypes.LifecycleState, ...userstypes.TransitionOption) (*userstypes.AuthUser, error) {
	return nil, nil
}
func (r *fakeAuthRepo) AllowedTransitions(context.Context, uuid.UUID) ([]userstypes.LifecycleTransition, error) {
	return nil, nil
}
func (r *fakeAuthRepo) ResetPassword(context.Context, uuid.UUID, string) error { return nil }

type fakeInventoryRepo struct {
	users []userstypes.AuthUser
}

func (r *fakeInventoryRepo) ListUsers(_ context.Context, filter userstypes.UserInventoryFilter) (userstypes.UserInventoryPage, error) {
	start := filter.Pagination.Offset
	if start > len(r.users) {
		start = len(r.users)
	}
	limit := filter.Pagination.Limit
	if limit <= 0 || start+limit > len(r.users) {
		limit = len(r.users) - start
	}
	items := append([]userstypes.AuthUser(nil), r.users[start:start+limit]...)
	next := start + limit
	return userstypes.UserInventoryPage{
		Users:      items,
		Total:      len(r.users),
		NextOffset: next,
		HasMore:    next < len(r.users),
	}, nil
}

type fakeProfileRepo struct {
	profiles map[string]userstypes.UserProfile
}

func (r *fakeProfileRepo) GetProfile(_ context.Context, userID uuid.UUID, scope userstypes.ScopeFilter) (*userstypes.UserProfile, error) {
	profile, ok := r.profiles[userID.String()+":"+scope.TenantID.String()]
	if !ok {
		return nil, nil
	}
	copy := profile
	return &copy, nil
}

func (r *fakeProfileRepo) UpsertProfile(context.Context, userstypes.UserProfile) (*userstypes.UserProfile, error) {
	return nil, nil
}

func TestRepositoryLoaderGetLoadsScopeAndProfile(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000010")
	authRepo := &fakeAuthRepo{
		users: map[uuid.UUID]userstypes.AuthUser{
			userID: {
				ID:       userID,
				Email:    "admin@example.com",
				Username: "admin",
				Status:   userstypes.LifecycleStateActive,
				Metadata: map[string]any{"tenant_id": tenantID.String()},
			},
		},
	}
	profileRepo := &fakeProfileRepo{
		profiles: map[string]userstypes.UserProfile{
			userID.String() + ":" + tenantID.String(): {
				UserID:      userID,
				DisplayName: "Admin User",
				Scope:       userstypes.ScopeFilter{TenantID: tenantID},
			},
		},
	}
	loader, err := NewRepositoryLoader(RepositoryLoaderConfig{
		Users:     authRepo,
		Inventory: &fakeInventoryRepo{},
		Profiles:  profileRepo,
		Logger:    noopLogger{},
		ResolveScope: func(_ context.Context, user userstypes.AuthUser, _ *userstypes.UserProfile) (userstypes.ScopeFilter, error) {
			return userstypes.ScopeFilter{TenantID: uuid.MustParse(user.Metadata["tenant_id"].(string))}, nil
		},
	})
	if err != nil {
		t.Fatalf("new loader: %v", err)
	}
	record, err := loader.Get(context.Background(), userID.String())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if record.Profile == nil || record.Scope.TenantID != tenantID {
		t.Fatalf("record = %#v", record)
	}
}

func TestRepositoryLoaderListSkipsRecordsWithoutScope(t *testing.T) {
	userID := uuid.New()
	missingScopeID := uuid.New()
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000010")
	users := []userstypes.AuthUser{
		{
			ID:       userID,
			Email:    "admin@example.com",
			Username: "admin",
			Status:   userstypes.LifecycleStateActive,
			Metadata: map[string]any{"tenant_id": tenantID.String()},
		},
		{
			ID:       missingScopeID,
			Email:    "missing@example.com",
			Username: "missing",
			Status:   userstypes.LifecycleStateActive,
		},
	}
	authRepo := &fakeAuthRepo{users: map[uuid.UUID]userstypes.AuthUser{
		userID:         users[0],
		missingScopeID: users[1],
	}}
	loader, err := NewRepositoryLoader(RepositoryLoaderConfig{
		Users:     authRepo,
		Inventory: &fakeInventoryRepo{users: users},
		Logger:    noopLogger{},
		ResolveScope: func(_ context.Context, user userstypes.AuthUser, _ *userstypes.UserProfile) (userstypes.ScopeFilter, error) {
			raw, _ := user.Metadata["tenant_id"].(string)
			if raw == "" {
				return userstypes.ScopeFilter{}, nil
			}
			return userstypes.ScopeFilter{TenantID: uuid.MustParse(raw)}, nil
		},
	})
	if err != nil {
		t.Fatalf("new loader: %v", err)
	}
	records, _, err := loader.List(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 1 || records[0].User.ID != userID {
		t.Fatalf("records = %#v", records)
	}
}

var _ searchtypes.Logger = noopLogger{}
