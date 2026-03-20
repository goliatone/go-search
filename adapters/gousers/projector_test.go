package gousers

import (
	"context"
	"testing"

	"github.com/google/uuid"
	userstypes "github.com/goliatone/go-users/pkg/types"
)

func TestUserProjectorBuildsScopedDocument(t *testing.T) {
	projector := NewUserProjector(UserProjectorConfig{Index: "users", SourceType: "user"})
	userID := uuid.New()
	docs, err := projector.Project(context.Background(), UserRecord{
		User: userstypes.AuthUser{
			ID:       userID,
			Role:     "admin",
			Status:   userstypes.LifecycleStateActive,
			Email:    "admin@example.com",
			Username: "admin",
		},
		Profile: &userstypes.UserProfile{
			UserID:      userID,
			DisplayName: "Admin User",
			Locale:      "en",
		},
		Scope: userstypes.ScopeFilter{
			TenantID: uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			OrgID:    uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		},
	})
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs = %#v", docs)
	}
	if docs[0].Scope.TenantID == "" || docs[0].Fields["role"] != "admin" {
		t.Fatalf("doc = %#v", docs[0])
	}
}
