package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func testServer(t *testing.T, agent http.HandlerFunc) *Server {
	t.Helper()
	backend := httptest.NewServer(agent)
	t.Cleanup(backend.Close)
	return &Server{
		store:    NewStore(),
		hub:      NewHub(),
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

func TestSnapshotKeepsEveryReportedControl(t *testing.T) {
	server := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"reply": "ok"})
	})
	elements := make([]map[string]string, 50)
	for i := range elements {
		elements[i] = map[string]string{"id": "e" + strconv.Itoa(i+1), "role": "link", "name": "Link", "tag": "a"}
	}
	raw, err := json.Marshal(map[string]any{
		"sessionId": "wide",
		"tabId":     4,
		"url":       "https://example.com",
		"title":     "Wide",
		"elements":  elements,
	})
	if err != nil {
		t.Fatal(err)
	}
	status, _ := do(server, http.MethodPost, "/v1/extension/snapshot", string(raw))
	if status != http.StatusOK {
		t.Fatalf("snapshot %d", status)
	}
	page := server.store.View("wide").Page
	if page == nil || len(page.Elements) != 50 {
		t.Fatalf("elements kept: %+v", page)
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
	do(server, http.MethodPost, "/v1/extension/snapshot", `{"tabId":3,"url":"https://example.com","title":"Account","elements":[{"id":"e3","name":"Delete account","role":"button","tag":"button"}]}`)
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
	do(server, http.MethodPost, "/v1/extension/snapshot", `{"tabId":4,"url":"https://example.com","title":"Account","elements":[]}`)
	status, payload := do(server, http.MethodPost, "/v1/chat", `{"message":"open it"}`)
	if status != http.StatusOK {
		t.Fatal(status, payload)
	}
	if _, payload = do(server, http.MethodGet, "/v1/extension/commands?sessionId=local", ""); strings.Contains(payload, "javascript") {
		t.Fatal(payload)
	}
}

func TestCommandUISelectsWhichTabsAreAvailable(t *testing.T) {
	server := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"reply":  "Click “Login”.",
			"action": map[string]string{"type": "click", "elementId": "e1", "text": "", "url": ""},
		})
	})
	status, payload := do(server, http.MethodPost, "/v1/extension/tabs", `{
		"sessionId":"local","activeTabId":2,
		"tabs":[
			{"tabId":1,"windowId":1,"url":"https://example.com/a","title":"Alpha","active":false,"elements":[]},
			{"tabId":2,"windowId":1,"url":"https://example.com/b","title":"Beta","active":true,"elements":[{"id":"e1","name":"Login","role":"button","tag":"button"}]}
		]
	}`)
	if status != http.StatusOK || !strings.Contains(payload, `"title":"Beta"`) {
		t.Fatalf("tabs %d %s", status, payload)
	}
	status, _ = do(server, http.MethodPost, "/v1/tabs/availability", `{"tabId":1,"available":false}`)
	if status != http.StatusOK {
		t.Fatalf("availability %d", status)
	}
	status, payload = do(server, http.MethodPost, "/v1/chat", `{"tabId":1,"message":"click Login"}`)
	if status != http.StatusConflict || !strings.Contains(payload, "not available") {
		t.Fatalf("disabled tab %d %s", status, payload)
	}
	status, payload = do(server, http.MethodPost, "/v1/chat", `{"tabId":2,"message":"click Login"}`)
	if status != http.StatusOK || !strings.Contains(payload, `"tabId":2`) {
		t.Fatalf("enabled tab %d %s", status, payload)
	}
	view := server.store.ViewTab("local", 2)
	if view.FocusTabID != 2 || len(view.Messages) == 0 || view.Messages[0].Content != "click Login" {
		t.Fatalf("transcript %+v", view.Messages)
	}
}

func TestSnapshotKeepsOtherTabTranscripts(t *testing.T) {
	server := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"reply":  "Click “Login”.",
			"action": map[string]string{"type": "click", "elementId": "e1", "text": "", "url": ""},
		})
	})
	status, payload := do(server, http.MethodPost, "/v1/extension/tabs", `{
		"sessionId":"local","activeTabId":586191126,
		"tabs":[{"tabId":586191126,"windowId":1,"url":"https://example.com","title":"Example","active":true,"elements":[{"id":"e1","name":"Login","role":"button","tag":"button"}]}]
	}`)
	if status != http.StatusOK {
		t.Fatalf("tabs %d %s", status, payload)
	}
	status, payload = do(server, http.MethodPost, "/v1/chat", `{"sessionId":"local","tabId":586191126,"message":"click Login"}`)
	if status != http.StatusOK || !strings.Contains(payload, "click Login") {
		t.Fatalf("chat %d %s", status, payload)
	}
	status, _ = do(server, http.MethodPost, "/v1/extension/snapshot", `{"sessionId":"local","tabId":1,"url":"http://127.0.0.1:7505/demo","title":"Practice page","elements":[]}`)
	if status != http.StatusOK {
		t.Fatalf("snapshot %d", status)
	}
	view := server.store.ViewTab("local", 586191126)
	if view.FocusTabID != 586191126 || len(view.Messages) < 2 || view.Messages[0].Content != "click Login" {
		t.Fatalf("transcript wiped: %+v tabs %+v", view.Messages, view.Tabs)
	}
	if len(view.Tabs) != 2 {
		t.Fatalf("tabs %+v", view.Tabs)
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

func TestWebsocketPushesSessionAndCommands(t *testing.T) {
	server := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"reply":  "Click “Login”.",
			"action": map[string]string{"type": "click", "elementId": "e1", "text": "", "url": ""},
		})
	})
	httpServer := httptest.NewServer(server.routes())
	t.Cleanup(httpServer.Close)
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/v1/ws?sessionId=local&role=extension"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var opened map[string]any
	if err := conn.ReadJSON(&opened); err != nil || opened["type"] != "session" {
		t.Fatalf("open %#v %v", opened, err)
	}
	status, payload := do(server, http.MethodPost, "/v1/extension/tabs", `{
		"sessionId":"local","activeTabId":9,
		"tabs":[{"tabId":9,"url":"https://example.com","title":"Example","active":true,"elements":[{"id":"e1","name":"Login","role":"button","tag":"button"}]}]
	}`)
	if status != http.StatusOK {
		t.Fatal(status, payload)
	}
	var pushed map[string]any
	if err := conn.ReadJSON(&pushed); err != nil || pushed["type"] != "session" {
		t.Fatalf("tabs push %#v %v", pushed, err)
	}
	status, payload = do(server, http.MethodPost, "/v1/chat", `{"sessionId":"local","tabId":9,"message":"click Login"}`)
	if status != http.StatusOK {
		t.Fatal(status, payload)
	}
	sawCommand := false
	for i := 0; i < 3 && !sawCommand; i++ {
		var msg struct {
			Type     string    `json:"type"`
			Commands []Command `json:"commands"`
		}
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
		if msg.Type == "commands" && len(msg.Commands) == 1 && msg.Commands[0].Type == "click" {
			sawCommand = true
		}
	}
	if !sawCommand {
		t.Fatal("command was not pushed")
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
