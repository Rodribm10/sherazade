package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/llm"
)

type stubSupportEvidenceProvider struct {
	request supportEvidenceRequest
	bundle  supportEvidenceBundle
	calls   int
}

func (s *stubSupportEvidenceProvider) Collect(_ context.Context, request supportEvidenceRequest) (supportEvidenceBundle, error) {
	s.calls++
	s.request = request
	return s.bundle, nil
}

func TestParseSupportAIResultEnforcesAutonomyGate(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantOutcome string
		wantPrefix  string
		wantError   bool
	}{
		{
			name:        "allows high-confidence low-risk guidance",
			raw:         `{"outcome":"answer","risk":"low","confidence":"high","reply":"Abra a tarefa e confira o prazo.","summary":"Orientação de uso"}`,
			wantOutcome: "answer",
		},
		{
			name:        "blocks a risky automatic answer",
			raw:         `{"outcome":"answer","risk":"high","confidence":"high","reply":"Vou corrigir a pontuação.","summary":"Alteração"}`,
			wantOutcome: "escalate",
			wantPrefix:  "Não fiz nenhuma alteração",
		},
		{
			name:        "does not duplicate the no-change notice",
			raw:         `{"outcome":"escalate","risk":"medium","confidence":"low","reply":"Não fiz nenhuma alteração no sistema. O caso precisa de investigação.","summary":"Escalonamento"}`,
			wantOutcome: "escalate",
			wantPrefix:  "Não fiz nenhuma alteração no sistema. O caso",
		},
		{
			name:      "rejects an unknown outcome",
			raw:       `{"outcome":"execute","risk":"low","confidence":"high","reply":"Feito.","summary":"Execução"}`,
			wantError: true,
		},
		{
			name:      "rejects model fields outside the contract",
			raw:       `{"outcome":"answer","risk":"low","confidence":"high","reply":"Confira o prazo.","summary":"Orientação","execute":true}`,
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := parseSupportAIResult(test.raw)
			if test.wantError {
				if err == nil {
					t.Fatal("expected parse error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSupportAIResult: %v", err)
			}
			if result.Outcome != test.wantOutcome {
				t.Fatalf("outcome=%q want=%q", result.Outcome, test.wantOutcome)
			}
			if test.wantPrefix != "" && !strings.HasPrefix(result.Reply, test.wantPrefix) {
				t.Fatalf("reply=%q want prefix=%q", result.Reply, test.wantPrefix)
			}
		})
	}
}

func newSupportAIHandlerTestCase(t *testing.T) (db.SupportCase, pgtype.UUID) {
	t.Helper()
	ctx := context.Background()
	agentID := chatTitleTestAgentID(t)
	session, err := testHandler.Queries.CreateChatSession(ctx, db.CreateChatSessionParams{
		WorkspaceID: parseUUID(testWorkspaceID),
		AgentID:     agentID,
		CreatorID:   parseUUID(testUserID),
		Title:       "Support AI test",
	})
	if err != nil {
		t.Fatalf("create chat session: %v", err)
	}
	message, err := testHandler.Queries.CreateChatMessage(ctx, db.CreateChatMessageParams{
		ChatSessionID: session.ID,
		Role:          "user",
		Content:       "Como confiro uma tarefa que não pontuou?",
	})
	if err != nil {
		t.Fatalf("create support message: %v", err)
	}
	sequence, err := testHandler.Queries.NextSupportCasePublicSequence(ctx, parseUUID(testWorkspaceID))
	if err != nil {
		t.Fatalf("allocate support sequence: %v", err)
	}
	caseRow, err := testHandler.Queries.CreateSupportCase(ctx, db.CreateSupportCaseParams{
		WorkspaceID:      parseUUID(testWorkspaceID),
		CaseNumber:       strconv.FormatInt(sequence, 10),
		ReporterUserID:   parseUUID(testUserID),
		ChatSessionID:    session.ID,
		PendingMessageID: message.ID,
		IdempotencyKey:   "support-ai-test-" + strings.TrimSpace(time.Now().Format("150405.000000000")),
	})
	if err != nil {
		t.Fatalf("create support case: %v", err)
	}
	if _, err := testHandler.Queries.CreateSupportCaseTransition(ctx, db.CreateSupportCaseTransitionParams{
		SupportCaseID: caseRow.ID,
		PreviousState: pgtype.Text{},
		NewState:      "novo",
		ActorType:     "member",
		ActorID:       parseUUID(testUserID),
	}); err != nil {
		t.Fatalf("create initial transition: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `
			DELETE FROM support_case_transition WHERE support_case_id = $1;
			DELETE FROM support_case WHERE id = $1;
			DELETE FROM chat_message WHERE chat_session_id = $2;
			DELETE FROM chat_session WHERE id = $2;
		`, caseRow.ID, session.ID)
	})
	return caseRow, agentID
}

func TestMaybeAnalyzeSupportCaseStoresReadOnlyAnswer(t *testing.T) {
	caseRow, agentID := newSupportAIHandlerTestCase(t)
	var llmRequestBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read LLM request: %v", err)
		}
		llmRequestBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		content := `{"outcome":"answer","risk":"low","confidence":"high","reply":"Abra a tarefa e confira se ela foi concluída dentro do prazo configurado.","summary":"Orientação sobre pontuação"}`
		_, _ = io.WriteString(w, `{"id":"cmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":`+jsonString(content)+`},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(srv.Close)
	evidence := &stubSupportEvidenceProvider{bundle: supportEvidenceBundle{Sources: []supportEvidenceSource{{
		Kind:       "data",
		Title:      "Explicação determinística da pontuação",
		Reference:  "rpc://explain_black_belt_score_v1",
		Content:    "A tarefa foi concluída depois do prazo configurado.",
		ObservedAt: "2026-08-01T16:00:01Z",
	}}}}
	handler := *testHandler
	handler.LLM = llm.New(llm.Config{APIKey: "test-key", BaseURL: srv.URL})
	handler.SupportEvidence = evidence

	completed := handler.maybeAnalyzeSupportCase(caseRow)
	if completed.State != "resposta_proposta" {
		t.Fatalf("state=%q", completed.State)
	}
	if completed.ResolutionType.String != "answer" || completed.RiskLevel.String != "low" || completed.Confidence.String != "high" {
		t.Fatalf("classification type=%q risk=%q confidence=%q", completed.ResolutionType.String, completed.RiskLevel.String, completed.Confidence.String)
	}
	if !sameSupportUUID(completed.LastAnsweredMessageID, caseRow.PendingMessageID) {
		t.Fatal("last answered message does not match the claimed input")
	}

	messages, err := testHandler.Queries.ListChatMessages(context.Background(), caseRow.ChatSessionID)
	if err != nil {
		t.Fatalf("list chat messages: %v", err)
	}
	if len(messages) != 2 || messages[1].Role != "assistant" || !strings.Contains(messages[1].Content, "Abra a tarefa") {
		t.Fatalf("messages=%+v", messages)
	}
	var tasks int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM agent_task_queue WHERE chat_session_id = $1`, caseRow.ChatSessionID).Scan(&tasks); err != nil {
		t.Fatalf("count agent tasks: %v", err)
	}
	if tasks != 0 {
		t.Fatalf("read-only Concierge enqueued %d coding task(s)", tasks)
	}
	if evidence.calls != 1 || evidence.request.ApplicationKey != "inaudit" || evidence.request.ReporterEmail == "" {
		t.Fatalf("evidence calls=%d request=%+v", evidence.calls, evidence.request)
	}
	if !strings.Contains(llmRequestBody, "rpc://explain_black_belt_score_v1") || !strings.Contains(llmRequestBody, `\"evidence_status\":\"complete\"`) {
		t.Fatalf("LLM request does not contain attributed evidence: %s", llmRequestBody)
	}
	var actorType, actorID, finalState string
	if err := testPool.QueryRow(context.Background(), `
		SELECT actor_type, actor_id::text, new_state
		FROM support_case_transition
		WHERE support_case_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, caseRow.ID).Scan(&actorType, &actorID, &finalState); err != nil {
		t.Fatalf("load final transition: %v", err)
	}
	if actorType != "agent" || actorID != uuidToString(agentID) || finalState != "resposta_proposta" {
		t.Fatalf("transition actor=%s/%s state=%s", actorType, actorID, finalState)
	}
}

func TestCompleteSupportAnalysisDropsStaleReply(t *testing.T) {
	caseRow, agentID := newSupportAIHandlerTestCase(t)
	claimed, ok, err := testHandler.claimSupportAnalysis(context.Background(), caseRow, agentID)
	if err != nil || !ok {
		t.Fatalf("claim support analysis: ok=%v err=%v", ok, err)
	}

	newerMessage, err := testHandler.Queries.CreateChatMessage(context.Background(), db.CreateChatMessageParams{
		ChatSessionID: caseRow.ChatSessionID,
		Role:          "user",
		Content:       "A tarefa é a BB-42 e foi concluída hoje.",
	})
	if err != nil {
		t.Fatalf("create newer support message: %v", err)
	}
	latest, err := testHandler.Queries.MarkSupportCasePending(context.Background(), db.MarkSupportCasePendingParams{
		PendingMessageID: newerMessage.ID,
		ID:               caseRow.ID,
		WorkspaceID:      caseRow.WorkspaceID,
	})
	if err != nil {
		t.Fatalf("mark newer message pending: %v", err)
	}

	_, applied, err := testHandler.completeSupportAnalysis(context.Background(), claimed, agentID, supportAIResult{
		Outcome:    "answer",
		Risk:       "low",
		Confidence: "high",
		Reply:      "Esta resposta já ficou desatualizada.",
		Summary:    "Resposta antiga",
	})
	if err != nil {
		t.Fatalf("complete stale support analysis: %v", err)
	}
	if applied {
		t.Fatal("stale support analysis was applied after a newer reporter message")
	}

	current, err := testHandler.Queries.GetSupportCaseForReporter(context.Background(), db.GetSupportCaseForReporterParams{
		ID: caseRow.ID, WorkspaceID: caseRow.WorkspaceID, ReporterUserID: caseRow.ReporterUserID,
	})
	if err != nil {
		t.Fatalf("load current support case: %v", err)
	}
	if current.State != latest.State || !sameSupportUUID(current.PendingMessageID, newerMessage.ID) {
		t.Fatalf("current state=%q pending=%s", current.State, uuidToString(current.PendingMessageID))
	}
	messages, err := testHandler.Queries.ListChatMessages(context.Background(), caseRow.ChatSessionID)
	if err != nil {
		t.Fatalf("list messages after stale completion: %v", err)
	}
	if len(messages) != 2 || messages[0].Role != "user" || messages[1].Role != "user" {
		t.Fatalf("stale completion persisted an assistant message: %+v", messages)
	}
}
