package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/auth"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type reporterDaemonFixture struct {
	userID      string
	workspaceID string
	email       string
	jwt         string
	pat         string
}

func newReporterDaemonFixture(t *testing.T) reporterDaemonFixture {
	t.Helper()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	fixture := reporterDaemonFixture{
		email: "support-reporter-daemon-" + suffix + "@example.test",
	}

	if err := testPool.QueryRow(ctx, `
		INSERT INTO "user" (name, email) VALUES ($1, $2) RETURNING id
	`, "Support Reporter Daemon", fixture.email).Scan(&fixture.userID); err != nil {
		t.Fatalf("create reporter user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, fixture.userID)
	})
	if err := testPool.QueryRow(ctx, `
		INSERT INTO workspace (name, slug, description) VALUES ($1, $2, $3) RETURNING id
	`, "Support Reporter Daemon Test", "support-reporter-daemon-"+suffix, "Temporary daemon authorization fixture").Scan(&fixture.workspaceID); err != nil {
		t.Fatalf("create reporter workspace: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, fixture.workspaceID)
	})
	if _, err := testPool.Exec(ctx, `
		INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'reporter')
	`, fixture.workspaceID, fixture.userID); err != nil {
		t.Fatalf("create reporter member: %v", err)
	}

	var err error
	fixture.jwt, err = generateTestJWT(fixture.userID, fixture.email, "Support Reporter Daemon")
	if err != nil {
		t.Fatalf("generate reporter JWT: %v", err)
	}
	fixture.pat = "mul_support_reporter_daemon_" + suffix
	if _, err := db.New(testPool).CreatePersonalAccessToken(ctx, db.CreatePersonalAccessTokenParams{
		UserID:      parseUUID(fixture.userID),
		Name:        "support reporter daemon test",
		TokenHash:   auth.HashToken(fixture.pat),
		TokenPrefix: fixture.pat[:12],
	}); err != nil {
		t.Fatalf("create reporter PAT: %v", err)
	}
	return fixture
}

func daemonRequest(t *testing.T, method, path, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, testServer.URL+path, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := testServer.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

func TestReporterIsDeniedFromDaemonWorkspaceEnumerationAndAccess(t *testing.T) {
	fixture := newReporterDaemonFixture(t)

	for name, token := range map[string]string{"jwt": fixture.jwt, "pat": fixture.pat} {
		t.Run(name+" workspace list", func(t *testing.T) {
			resp := daemonRequest(t, http.MethodGet, "/api/daemon/workspaces", token)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
			var workspaces []map[string]string
			if err := json.NewDecoder(resp.Body).Decode(&workspaces); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if len(workspaces) != 0 {
				t.Fatalf("reporter daemon workspace list = %#v, want empty", workspaces)
			}
		})

		t.Run(name+" workspace access", func(t *testing.T) {
			resp := daemonRequest(t, http.MethodGet, "/api/daemon/workspaces/"+fixture.workspaceID+"/repos", token)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
			}
		})
	}
}

func TestDaemonTokenRetainsBoundWorkspaceAccess(t *testing.T) {
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var workspaceID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO workspace (name, slug, description) VALUES ($1, $2, $3) RETURNING id
	`, "Daemon Token Access Test", "daemon-token-access-"+suffix, "Temporary daemon token fixture").Scan(&workspaceID); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, workspaceID)
	})

	token, err := auth.GenerateDaemonToken()
	if err != nil {
		t.Fatalf("generate daemon token: %v", err)
	}
	if _, err := db.New(testPool).CreateDaemonToken(ctx, db.CreateDaemonTokenParams{
		TokenHash:   auth.HashToken(token),
		WorkspaceID: parseUUID(workspaceID),
		DaemonID:    "support-reporter-daemon-test-" + suffix,
		ExpiresAt:   pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("create daemon token: %v", err)
	}

	resp := daemonRequest(t, http.MethodGet, "/api/daemon/workspaces", token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var workspaces []map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&workspaces); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(workspaces) != 1 || workspaces[0]["id"] != workspaceID {
		t.Fatalf("daemon workspace list = %#v, want bound workspace %s", workspaces, workspaceID)
	}

	resp = daemonRequest(t, http.MethodGet, "/api/daemon/workspaces/"+workspaceID+"/repos", token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("workspace repos status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestReporterIsDeniedFromOnboardingBootstrapShims(t *testing.T) {
	fixture := newReporterDaemonFixture(t)
	cases := []struct {
		name string
		path string
		body string
	}{
		{"runtime", "/api/me/onboarding/runtime-bootstrap", `{"workspace_id":"` + fixture.workspaceID + `","runtime_id":"00000000-0000-0000-0000-000000000001"}`},
		{"no runtime", "/api/me/onboarding/no-runtime-bootstrap", `{"workspace_id":"` + fixture.workspaceID + `"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, testServer.URL+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("create request: %v", err)
			}
			req.Header.Set("Authorization", "Bearer "+fixture.jwt)
			req.Header.Set("Content-Type", "application/json")
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
