package types

import (
	"context"
	"testing"
)

func TestDefaultScopeGuardFailsClosedForScopedRequestsAndDocuments(t *testing.T) {
	guard := DefaultScopeGuard{}
	req := SearchRequest{Scope: Scope{TenantID: "tenant-a"}}
	if guard.AllowSearch(context.Background(), ActorRef{}, req) {
		t.Fatal("anonymous actor was allowed to select a tenant scope")
	}
	actor := ActorRef{UserID: "user-1", TenantID: "tenant-a"}
	if !guard.AllowSearch(context.Background(), actor, req) {
		t.Fatal("matching authenticated actor was denied")
	}
	doc := Document{Scope: Scope{TenantID: "tenant-b"}}
	if guard.AllowDocument(context.Background(), actor, doc) {
		t.Fatal("actor was allowed to read a different tenant")
	}
}

func TestDefaultScopeGuardEnforcesVisibility(t *testing.T) {
	guard := DefaultScopeGuard{}
	private := Document{Visibility: Visibility{Roles: []string{"admin"}}}
	if guard.AllowDocument(context.Background(), ActorRef{}, private) {
		t.Fatal("anonymous actor was allowed to read a private document")
	}
	admin := ActorRef{UserID: "admin-1", Metadata: map[string]any{"role": "admin"}}
	if !guard.AllowDocument(context.Background(), admin, private) {
		t.Fatal("allowed role was denied")
	}
	if !guard.AllowDocument(context.Background(), ActorRef{}, Document{}) {
		t.Fatal("legacy unconstrained document should remain public")
	}
}
