package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testServer(t *testing.T, agent http.HandlerFunc) *Server {
	t.Helper()
	backend := httptest.NewServer(agent)
	t.Cleanup(backend.Close)
	return &Server{
		store:    NewStore(),
		agentURL: backend.URL,
		client:   &http.Client{Timeout: 5 * time.Second},
	}
}

func TestChatEnqueuesClickFromBrain(t *testing.T) {
	server := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/decide" {
			t.Fatalf("path %s", r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"reply":             "Click “Login”.",
			"backend":           "laya",
			"needsConfirmation": false,
			"action":            map[string]string{"type": "click", "elementId": "e1", "text": "", "url": ""},
			"decision":          map[string]any{"operation": "click", "target": "e1"},
		})
	})
	body := `{"sessionId":"local","tabId":7,"url":"https://example.com","title":"Account","elements":[{"id":"e1","role":"button","name":"Login","tag":"button"}]}`
	status, _ := do(server, http.MethodPost, "/v1/extension/snapshot", body)
	if status != http.StatusOK {
		t.Fatalf("snapshot %d", status)
	}
	status, payload := do(server, http.MethodPost, "/v1/chat", `{"sessionId":"local","message":"click Login"}`)
	if status != http.StatusOK {
		t.Fatalf("chat %d %s", status, payload)
	}
	status, payload = do(server, http.MethodGet, "/v1/extension/commands?sessionId=local", "")
	var queued struct {
		Commands []Command `json:"commands"`
	}
	if err := json.Unmarshal([]byte(payload), &queued); err != nil {
		t.Fatal(err)
	}
	if len(queued.Commands) != 1 || queued.Commands[0].Type != "click" || queued.Commands[0].TabID != 7 {
		t.Fatalf("commands %+v", queued.Commands)
	}
	status, _ = do(server, http.MethodPost, "/v1/extension/result", `{"sessionId":"local","commandId":"`+queued.Commands[0].ID+`","ok":true,"detail":"Clicked Login."}`)
	if status != http.StatusOK {
		t.Fatalf("result %d", status)
	}
	view := server.store.View("local")
	if view.LastResult == nil || view.LastResult.Detail != "Clicked Login." {
		t.Fatalf("result not stored: %+v", view.LastResult)
	}
}

func TestConfirmationIsHeldUntilReleased(t *testing.T) {
	server := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"reply":             "Click “Delete account”. Confirm and I will run it.",
			"backend":           "heuristic",
			"needsConfirmation": true,
			"action":            map[string]string{"type": "click", "elementId": "e3", "text": "", "url": ""},
		})
	})
	status, payload := do(server, http.MethodPost, "/v1/chat", `{"message":"click Delete account"}`)
	if status != http.StatusOK || !strings.Contains(payload, `"needsConfirmation":true`) {
		t.Fatalf("chat %d %s", status, payload)
	}
	status, payload = do(server, http.MethodGet, "/v1/extension/commands?sessionId=local", "")
	if strings.Contains(payload, "e3") {
		t.Fatalf("held action was queued: %s", payload)
	}
	status, payload = do(server, http.MethodPost, "/v1/chat", `{"confirmed":true}`)
	if status != http.StatusOK || !strings.Contains(payload, `"type":"click"`) {
		t.Fatalf("confirm %d %s", status, payload)
	}
}

func TestRejectsNavigateWithoutHTTP(t *testing.T) {
	server := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"reply":  "Open javascript:alert(1).",
			"action": map[string]string{"type": "navigate", "elementId": "", "text": "", "url": "javascript:alert(1)"},
		})
	})
	status, payload := do(server, http.MethodPost, "/v1/chat", `{"message":"open it"}`)
	if status != http.StatusOK {
		t.Fatal(status, payload)
	}
	if _, payload = do(server, http.MethodGet, "/v1/extension/commands?sessionId=local", ""); strings.Contains(payload, "javascript") {
		t.Fatal(payload)
	}
}

func TestTokenRequiredWhenConfigured(t *testing.T) {
	server := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	server.token = "secret"
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", res.Code)
	}
	req.Header.Set("X-Api-Token", "secret")
	res = httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("authed status %d", res.Code)
	}
}

func do(server *Server, method, path, body string) (int, string) {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	return res.Code, res.Body.String()
}
