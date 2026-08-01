package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestSupportSessionsAreReporterOwnedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var workspaceID, conciergeRuntimeID, conciergeID, reporterOne, reporterTwo string
	if err := testPool.QueryRow(ctx, `INSERT INTO workspace (name, slug, description) VALUES ($1,$2,$3) RETURNING id`, "Support sessions", "support-sessions-"+suffix, "test").Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, workspaceID) })
	for i, destination := range []*string{&reporterOne, &reporterTwo} {
		if err := testPool.QueryRow(ctx, `INSERT INTO "user" (name,email) VALUES ($1,$2) RETURNING id`, fmt.Sprintf("Reporter %d", i), fmt.Sprintf("support-session-%d-%d@example.test", i, time.Now().UnixNano())).Scan(destination); err != nil {
			t.Fatal(err)
		}
		if _, err := testPool.Exec(ctx, `INSERT INTO member (workspace_id,user_id,role) VALUES ($1,$2,'reporter')`, workspaceID, *destination); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM "user" WHERE id IN ($1,$2)`, reporterOne, reporterTwo)
	})
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent_runtime (
			workspace_id, daemon_id, name, runtime_mode, provider, status,
			device_info, metadata, owner_id, last_seen_at
		)
		VALUES ($1, NULL, $2, 'cloud', 'support_test_runtime', 'online', $3, '{}'::jsonb, $4, now())
		RETURNING id
	`, workspaceID, "Support Test Runtime", "Support test runtime", reporterOne).Scan(&conciergeRuntimeID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `INSERT INTO agent (workspace_id,name,description,runtime_mode,runtime_config,runtime_id,visibility,permission_mode,max_concurrent_tasks,owner_id) VALUES ($1,$2,'','cloud','{}',$3,'workspace','public_to',1,$4) RETURNING id`, workspaceID, "Support Concierge", conciergeRuntimeID, reporterOne).Scan(&conciergeID); err != nil {
		t.Fatal(err)
	}

	tokenOne, err := generateTestJWT(reporterOne, "one@example.test", "Reporter One")
	if err != nil {
		t.Fatalf("generate reporter one JWT: %v", err)
	}
	post := func(token, key string) *http.Response {
		req, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/support/sessions?workspace_id="+workspaceID, bytes.NewBufferString(`{"idempotency_key":"`+key+`","description":"Preciso de ajuda"}`))
		if err != nil {
			t.Fatalf("create POST request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := testServer.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	get := func(token, path, targetWorkspace string) *http.Response {
		req, err := http.NewRequest(http.MethodGet, testServer.URL+path+"?workspace_id="+targetWorkspace, nil)
		if err != nil {
			t.Fatalf("create GET request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		response, err := testServer.Client().Do(req)
		if err != nil {
			t.Fatalf("GET request: %v", err)
		}
		return response
	}
	postMessage := func(token, sessionID, targetWorkspace, body string) *http.Response {
		req, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/support/sessions/"+sessionID+"/messages?workspace_id="+targetWorkspace, bytes.NewBufferString(body))
		if err != nil {
			t.Fatalf("create support message request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		response, err := testServer.Client().Do(req)
		if err != nil {
			t.Fatalf("support message request: %v", err)
		}
		return response
	}
	resp := get(tokenOne, "/api/workspaces", workspaceID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list workspaces status=%d", resp.StatusCode)
	}
	var workspaceList []struct {
		ID   string `json:"id"`
		Role string `json:"role"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&workspaceList); err != nil {
		t.Fatalf("decode workspace list: %v", err)
	}
	resp.Body.Close()
	foundReporterRole := false
	for _, workspace := range workspaceList {
		if workspace.ID == workspaceID && workspace.Role == "reporter" {
			foundReporterRole = true
			break
		}
	}
	if !foundReporterRole {
		t.Fatalf("workspace list did not expose reporter role: %+v", workspaceList)
	}
	resp = post(tokenOne, "idem-key-0001")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("missing config status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	if _, err := testPool.Exec(ctx, `UPDATE workspace SET settings = jsonb_build_object('support', jsonb_build_object('concierge_agent_id',$2::text)) WHERE id=$1`, workspaceID, conciergeID); err != nil {
		t.Fatal(err)
	}
	resp = post(tokenOne, "idem-key-0001")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d", resp.StatusCode)
	}
	var created map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created response: %v", err)
	}
	resp.Body.Close()
	if created["public_code"] != "SUP-000001" {
		t.Fatalf("code=%q", created["public_code"])
	}
	var previousState *string
	var newState, transitionActor string
	if err := testPool.QueryRow(ctx, `SELECT previous_state, new_state, actor_user_id::text FROM support_case_transition WHERE support_case_id=$1`, created["id"]).Scan(&previousState, &newState, &transitionActor); err != nil {
		t.Fatalf("read initial transition: %v", err)
	}
	if previousState != nil || newState != "novo" || transitionActor != reporterOne {
		t.Fatalf("initial transition previous=%v new=%q actor=%q", previousState, newState, transitionActor)
	}
	for _, role := range []string{"owner", "admin", "member"} {
		var userID string
		if err := testPool.QueryRow(ctx, `INSERT INTO "user" (name,email) VALUES ($1,$2) RETURNING id`, role, role+"-support-"+suffix+"@example.test").Scan(&userID); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = testPool.Exec(context.Background(), `DELETE FROM "user" WHERE id=$1`, userID) })
		if _, err := testPool.Exec(ctx, `INSERT INTO member (workspace_id,user_id,role) VALUES ($1,$2,$3)`, workspaceID, userID, role); err != nil {
			t.Fatal(err)
		}
		token, err := generateTestJWT(userID, role+"@example.test", role)
		if err != nil {
			t.Fatalf("generate %s JWT: %v", role, err)
		}
		resp = get(token, "/api/support/sessions", workspaceID)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("%s support status=%d", role, resp.StatusCode)
		}
		resp.Body.Close()
	}
	resp = get(tokenOne, "/api/support/sessions", workspaceID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status=%d", resp.StatusCode)
	}
	var listed []map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	resp.Body.Close()
	if len(listed) != 1 || listed[0]["id"] != created["id"] {
		t.Fatalf("list=%v", listed)
	}
	resp = get(tokenOne, "/api/support/sessions/"+created["session_id"], workspaceID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get session status=%d", resp.StatusCode)
	}
	var fetchedSession map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&fetchedSession); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	resp.Body.Close()
	if fetchedSession["id"] != created["id"] {
		t.Fatalf("session returned case %q", fetchedSession["id"])
	}
	resp = get(tokenOne, "/api/support/sessions/"+created["session_id"]+"/messages", workspaceID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list support messages status=%d", resp.StatusCode)
	}
	var messages []struct {
		ID      string `json:"id"`
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
		t.Fatalf("decode support messages: %v", err)
	}
	resp.Body.Close()
	if len(messages) != 1 || messages[0].Role != "user" || messages[0].Content != "Preciso de ajuda" {
		t.Fatalf("initial support messages=%+v", messages)
	}
	resp = postMessage(tokenOne, created["session_id"], workspaceID, `{"content":"Mais contexto"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("send support message status=%d", resp.StatusCode)
	}
	var sentMessage struct {
		ID      string `json:"id"`
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sentMessage); err != nil {
		t.Fatalf("decode sent support message: %v", err)
	}
	resp.Body.Close()
	if sentMessage.ID == "" || sentMessage.Role != "user" || sentMessage.Content != "Mais contexto" {
		t.Fatalf("sent support message=%+v", sentMessage)
	}
	var enqueuedTasks int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM agent_task_queue WHERE chat_session_id=$1`, created["session_id"]).Scan(&enqueuedTasks); err != nil {
		t.Fatalf("count support tasks: %v", err)
	}
	if enqueuedTasks != 0 {
		t.Fatalf("support message unexpectedly enqueued %d task(s)", enqueuedTasks)
	}
	resp = postMessage(tokenOne, created["session_id"], workspaceID, `{"content":"","agent_id":"`+conciergeID+`"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("support message accepted unknown field status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = post(tokenOne, "idem-key-0001")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("retry status=%d", resp.StatusCode)
	}
	var retried map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&retried); err != nil {
		t.Fatalf("decode retry response: %v", err)
	}
	resp.Body.Close()
	if retried["id"] != created["id"] {
		t.Fatal("retry created a second case")
	}
	var otherWorkspaceID string
	if err := testPool.QueryRow(ctx, `INSERT INTO workspace (name, slug, description) VALUES ($1,$2,$3) RETURNING id`, "Other support sessions", "other-support-sessions-"+suffix, "test").Scan(&otherWorkspaceID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, otherWorkspaceID)
	})
	if _, err := testPool.Exec(ctx, `INSERT INTO member (workspace_id,user_id,role) VALUES ($1,$2,'reporter')`, otherWorkspaceID, reporterOne); err != nil {
		t.Fatal(err)
	}
	resp = get(tokenOne, "/api/support/cases/"+created["id"], otherWorkspaceID)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-workspace status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = get(tokenOne, "/api/support/sessions", otherWorkspaceID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cross-workspace list status=%d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode cross-workspace list: %v", err)
	}
	resp.Body.Close()
	if len(listed) != 0 {
		t.Fatalf("cross-workspace list leaked %v", listed)
	}
	resp = get(tokenOne, "/api/support/sessions/"+created["session_id"], otherWorkspaceID)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-workspace session status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = get(tokenOne, "/api/support/sessions/"+created["session_id"]+"/messages", otherWorkspaceID)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-workspace messages status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = postMessage(tokenOne, created["session_id"], otherWorkspaceID, `{"content":"Tentativa externa"}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-workspace send message status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	tokenTwo, err := generateTestJWT(reporterTwo, "two@example.test", "Reporter Two")
	if err != nil {
		t.Fatalf("generate reporter two JWT: %v", err)
	}
	resp = get(tokenTwo, "/api/support/cases/"+created["id"], workspaceID)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-reporter status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = get(tokenTwo, "/api/support/sessions/"+created["session_id"], workspaceID)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-reporter session status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = get(tokenTwo, "/api/support/sessions/"+created["session_id"]+"/messages", workspaceID)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-reporter messages status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = postMessage(tokenTwo, created["session_id"], workspaceID, `{"content":"Tentativa de outra pessoa"}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-reporter send message status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = get(tokenTwo, "/api/support/sessions", workspaceID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cross-reporter list status=%d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode cross-reporter list: %v", err)
	}
	resp.Body.Close()
	if len(listed) != 0 {
		t.Fatalf("cross-reporter list leaked %v", listed)
	}
	resp = post(tokenOne, "idem-key-0002")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("second create status=%d", resp.StatusCode)
	}
	var second map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&second); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	resp.Body.Close()
	if second["public_code"] != "SUP-000002" {
		t.Fatalf("second code=%q", second["public_code"])
	}
	start := make(chan struct{})
	statuses := make(chan int, 2)
	errs := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			req, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/support/sessions?workspace_id="+workspaceID, bytes.NewBufferString(`{"idempotency_key":"idem-key-concurrent","description":"Preciso de ajuda"}`))
			if err != nil {
				errs <- err
				return
			}
			req.Header.Set("Authorization", "Bearer "+tokenOne)
			req.Header.Set("Content-Type", "application/json")
			response, err := testServer.Client().Do(req)
			if err != nil {
				errs <- err
				return
			}
			defer response.Body.Close()
			statuses <- response.StatusCode
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	close(statuses)
	createdCount, retriedCount := 0, 0
	for status := range statuses {
		if status == http.StatusCreated {
			createdCount++
		} else if status == http.StatusOK {
			retriedCount++
		} else {
			t.Fatalf("concurrent status=%d", status)
		}
	}
	if createdCount != 1 || retriedCount != 1 {
		t.Fatalf("concurrent statuses created=%d retried=%d", createdCount, retriedCount)
	}
	var caseCount, sessionCount, transitionCount int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM support_case WHERE workspace_id=$1 AND reporter_user_id=$2 AND idempotency_key='idem-key-concurrent'`, workspaceID, reporterOne).Scan(&caseCount); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM chat_session s JOIN support_case c ON c.chat_session_id=s.id WHERE c.workspace_id=$1 AND c.reporter_user_id=$2 AND c.idempotency_key='idem-key-concurrent'`, workspaceID, reporterOne).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM support_case_transition t JOIN support_case c ON c.id=t.support_case_id WHERE c.workspace_id=$1 AND c.reporter_user_id=$2 AND c.idempotency_key='idem-key-concurrent'`, workspaceID, reporterOne).Scan(&transitionCount); err != nil {
		t.Fatal(err)
	}
	if caseCount != 1 || sessionCount != 1 || transitionCount != 1 {
		t.Fatalf("concurrent rows cases=%d support sessions=%d transitions=%d", caseCount, sessionCount, transitionCount)
	}
	if _, err := testPool.Exec(ctx, `UPDATE support_case_sequence SET next_value=999999 WHERE workspace_id=$1`, workspaceID); err != nil {
		t.Fatalf("set support code boundary: %v", err)
	}
	resp = post(tokenOne, "idem-key-boundary")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("boundary create status=%d", resp.StatusCode)
	}
	var boundary map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&boundary); err != nil {
		t.Fatalf("decode boundary response: %v", err)
	}
	resp.Body.Close()
	if boundary["public_code"] != "SUP-1000000" {
		t.Fatalf("boundary code=%q", boundary["public_code"])
	}
	req, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/support/sessions?workspace_id="+workspaceID, bytes.NewBufferString(`{"idempotency_key":"idem-key-unknown","description":"Preciso de ajuda","agent_id":"`+conciergeID+`"}`))
	if err != nil {
		t.Fatalf("create unknown-field request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+tokenOne)
	req.Header.Set("Content-Type", "application/json")
	resp, err = testServer.Client().Do(req)
	if err != nil {
		t.Fatalf("unknown-field request: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("client agent_id status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	var foreignRuntimeID, foreignAgentID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent_runtime (
			workspace_id, daemon_id, name, runtime_mode, provider, status,
			device_info, metadata, owner_id, last_seen_at
		)
		VALUES ($1, NULL, $2, 'cloud', 'support_foreign_test_runtime', 'online', $3, '{}'::jsonb, $4, now())
		RETURNING id
	`, otherWorkspaceID, "Foreign Support Test Runtime", "Foreign support test runtime", reporterOne).Scan(&foreignRuntimeID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `INSERT INTO agent (workspace_id,name,description,runtime_mode,runtime_config,runtime_id,visibility,permission_mode,max_concurrent_tasks,owner_id) VALUES ($1,$2,'','cloud','{}',$3,'workspace','public_to',1,$4) RETURNING id`, otherWorkspaceID, "Foreign Concierge", foreignRuntimeID, reporterOne).Scan(&foreignAgentID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `UPDATE workspace SET settings = jsonb_build_object('support', jsonb_build_object('concierge_agent_id',$2::text)) WHERE id=$1`, workspaceID, foreignAgentID); err != nil {
		t.Fatal(err)
	}
	resp = post(tokenOne, "idem-key-0003")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("cross-workspace concierge status=%d", resp.StatusCode)
	}
	resp.Body.Close()
}
