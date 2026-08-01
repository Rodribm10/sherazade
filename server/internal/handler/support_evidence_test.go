package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewHTTPSupportEvidenceClientRejectsInsecureRemoteEndpoint(t *testing.T) {
	client, err := newHTTPSupportEvidenceClient("http://evidence.example.com/v1/investigate", "", time.Second)
	if err == nil || client != nil {
		t.Fatal("expected insecure remote endpoint to be rejected")
	}
}

func TestHTTPSupportEvidenceClientCollectsBoundedAttributedEvidence(t *testing.T) {
	var received supportEvidenceRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer broker-token" {
			t.Errorf("authorization=%q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"sources":[
				{"kind":"code","title":"Regra implantada","reference":"github://inaudit/src/score.ts:42","content":"A tarefa pontua somente quando status=done.","deployed_revision":"abc123"},
				{"kind":"data","title":"Estado atual","reference":"rpc://explain_black_belt_score_v1","content":"Authorization: Bearer very-secret-token-value","observed_at":"2026-08-01T16:00:01Z"}
			],
			"limitations":["A sincronização externa ainda não foi consultada."]
		}`)
	}))
	t.Cleanup(server.Close)

	client, err := newHTTPSupportEvidenceClient(server.URL, "broker-token", 2*time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	bundle, err := client.Collect(context.Background(), supportEvidenceRequest{
		CaseID:         "case-1",
		WorkspaceID:    "workspace-1",
		ReporterUserID: "user-1",
		ReporterEmail:  "lider@example.com",
		ReporterName:   "Lider Teste",
		ApplicationKey: "inaudit",
		Conversation: []supportPromptMessage{{
			Role:    "user",
			Content: "Por que minha tarefa não pontuou?",
		}},
	})
	if err != nil {
		t.Fatalf("collect evidence: %v", err)
	}
	if received.ApplicationKey != "inaudit" || received.ReporterEmail != "lider@example.com" || len(received.Conversation) != 1 {
		t.Fatalf("received request=%+v", received)
	}
	if len(bundle.Sources) != 2 || bundle.Sources[0].Reference != "github://inaudit/src/score.ts:42" {
		t.Fatalf("sources=%+v", bundle.Sources)
	}
	if strings.Contains(bundle.Sources[1].Content, "very-secret-token-value") {
		t.Fatal("evidence response retained a bearer token")
	}
	if len(bundle.Limitations) != 1 {
		t.Fatalf("limitations=%+v", bundle.Limitations)
	}
}

func TestHTTPSupportEvidenceClientDoesNotFollowRedirects(t *testing.T) {
	targetCalled := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalled = true
		_, _ = io.WriteString(w, `{"sources":[]}`)
	}))
	t.Cleanup(target.Close)
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirect.Close)

	client, err := newHTTPSupportEvidenceClient(redirect.URL, "must-not-leak", time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = client.Collect(context.Background(), supportEvidenceRequest{})
	if err == nil {
		t.Fatal("expected redirect response to fail")
	}
	if targetCalled {
		t.Fatal("evidence client followed a redirect and could have leaked its token")
	}
}

func TestNormalizeSupportEvidenceRequiresFreshnessAndDeployedRevision(t *testing.T) {
	tests := []struct {
		name   string
		source supportEvidenceSource
	}{
		{
			name: "code without deployed revision",
			source: supportEvidenceSource{
				Kind:      "code",
				Reference: "github://inaudit/src/score.ts:42",
				Content:   "Regra encontrada.",
			},
		},
		{
			name: "data without observation timestamp",
			source: supportEvidenceSource{
				Kind:      "data",
				Reference: "rpc://explain_black_belt_score_v1",
				Content:   "Resultado encontrado.",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := normalizeSupportEvidence(supportEvidenceBundle{Sources: []supportEvidenceSource{test.source}})
			if err == nil {
				t.Fatal("expected incomplete source provenance to be rejected")
			}
		})
	}
}
