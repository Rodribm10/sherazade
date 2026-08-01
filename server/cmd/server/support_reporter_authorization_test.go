package main

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestReporterIsDeniedFromTechnicalWorkspaceRoutes(t *testing.T) {
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	email := "support-reporter-" + suffix + "@example.test"
	slug := "support-reporter-" + suffix

	var userID, workspaceID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO "user" (name, email) VALUES ($1, $2) RETURNING id
	`, "Support Reporter", email).Scan(&userID); err != nil {
		t.Fatalf("create reporter user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, userID)
	})
	if err := testPool.QueryRow(ctx, `
		INSERT INTO workspace (name, slug, description) VALUES ($1, $2, $3) RETURNING id
	`, "Support Reporter Test", slug, "Temporary reporter authorization fixture").Scan(&workspaceID); err != nil {
		t.Fatalf("create reporter workspace: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, workspaceID)
	})
	if _, err := testPool.Exec(ctx, `
		INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'reporter')
	`, workspaceID, userID); err != nil {
		t.Fatalf("create reporter member: %v", err)
	}

	token, err := generateTestJWT(userID, email, "Support Reporter")
	if err != nil {
		t.Fatalf("generate reporter token: %v", err)
	}

	paths := []string{
		"/api/issues?workspace_id=" + workspaceID,
		"/api/agents?workspace_id=" + workspaceID,
		"/api/projects?workspace_id=" + workspaceID,
		"/api/skills?workspace_id=" + workspaceID,
		"/api/runtimes?workspace_id=" + workspaceID,
		"/api/dashboard/usage/daily?workspace_id=" + workspaceID,
		"/api/chat/sessions?workspace_id=" + workspaceID,
		"/api/workspaces/" + workspaceID,
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, testServer.URL+path, nil)
			if err != nil {
				t.Fatalf("create request: %v", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := testServer.Client().Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
			}
		})
	}
}

func TestReporterCannotSubscribeToRealtime(t *testing.T) {
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	email := "support-reporter-realtime-" + suffix + "@example.test"
	slug := "support-reporter-realtime-" + suffix

	var userID, workspaceID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO "user" (name, email) VALUES ($1, $2) RETURNING id
	`, "Support Reporter Realtime", email).Scan(&userID); err != nil {
		t.Fatalf("create reporter user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, userID)
	})
	if err := testPool.QueryRow(ctx, `
		INSERT INTO workspace (name, slug, description) VALUES ($1, $2, $3) RETURNING id
	`, "Support Reporter Realtime Test", slug, "Temporary realtime authorization fixture").Scan(&workspaceID); err != nil {
		t.Fatalf("create reporter workspace: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, workspaceID)
	})
	if _, err := testPool.Exec(ctx, `
		INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'reporter')
	`, workspaceID, userID); err != nil {
		t.Fatalf("create reporter member: %v", err)
	}

	checker := &membershipChecker{queries: db.New(testPool)}
	if checker.IsMember(ctx, userID, workspaceID) {
		t.Fatal("reporter must not be allowed to subscribe to workspace realtime")
	}
}
