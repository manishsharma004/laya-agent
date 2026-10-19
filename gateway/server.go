package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
)

//go:embed demo.html
var demoHTML []byte

var sessionPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// maxPageElements is how many links, buttons, and selectables one tab may report.
const maxPageElements = 200

func trimElements(elements []Element) []Element {
	if len(elements) > maxPageElements {
		return elements[:maxPageElements]
	}
	return elements
}

func trimOptionalElements(elements *[]Element) *[]Element {
	if elements == nil {
		return nil
	}
	trimmed := trimElements(*elements)
	return &trimmed
}

var allowedActions = map[string]bool{
	"click": true, "type": true, "select": true, "scroll": true, "navigate": true, "extract": true,
}

type Server struct {
	store    *Store
	hub      *Hub
	agentURL string
	token    string
	client   *http.Client
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /v1/status", s.handleStatus)
	mux.HandleFunc("POST /v1/extension/snapshot", s.handleSnapshot)
	mux.HandleFunc("POST /v1/extension/tabs", s.handleTabs)
	mux.HandleFunc("POST /v1/tabs/availability", s.handleAvailability)
	mux.HandleFunc("GET /v1/extension/commands", s.handleCommands)
	mux.HandleFunc("POST /v1/extension/result", s.handleResult)
	mux.HandleFunc("POST /v1/chat", s.handleChat)
	mux.HandleFunc("GET /v1/sessions/{id}", s.handleSession)
	mux.HandleFunc("GET /v1/ws", s.handleWS)
	mux.HandleFunc("GET /demo", s.handleDemo)
	mux.HandleFunc("GET /", s.handleRoot)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if cors(w, r) {
			return
		}
		if !s.authorized(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing or invalid API token"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func cors(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Api-Token")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

func (s *Server) authorized(r *http.Request) bool {
	if s.token == "" || r.URL.Path == "/health" || r.URL.Path == "/demo" {
		return true
	}
	if r.URL.Path == "/v1/ws" && r.URL.Query().Get("token") == s.token {
		return true
	}
	return r.Header.Get("X-Api-Token") == s.token
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "laya-gateway",
		"demo":    "/demo",
		"routes":  []string{"/health", "/v1/status", "/v1/ws", "/v1/chat", "/v1/sessions/{id}", "/v1/extension/tabs", "/v1/tabs/availability", "/v1/extension/commands", "/v1/extension/result"},
	})
}

func (s *Server) handleDemo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(demoHTML)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	agent := map[string]any{"ok": false}
	status := http.StatusOK
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(s.agentURL, "/")+"/health", nil)
	if err == nil {
		resp, err := s.client.Do(req)
		if err != nil {
			agent["error"] = err.Error()
		} else {
			defer resp.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			if resp.StatusCode >= 300 || json.Unmarshal(body, &agent) != nil {
				agent = map[string]any{"ok": false, "error": strings.TrimSpace(string(body))}
			}
		}
	}
	writeJSON(w, status, map[string]any{"api": "ok", "agent": agent, "sessions": s.store.SessionCount()})
}

func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SessionID string    `json:"sessionId"`
		TabID     int       `json:"tabId"`
		URL       string    `json:"url"`
		Title     string    `json:"title"`
		Elements  []Element `json:"elements"`
	}
	if !decode(w, r, &body) {
		return
	}
	id := sessionID(body.SessionID)
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sessionId"})
		return
	}
	body.Elements = trimElements(body.Elements)
	s.store.SavePage(id, Page{URL: body.URL, Title: body.Title, Elements: body.Elements, TabID: body.TabID})
	s.publishSession(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "elements": len(body.Elements), "session": s.store.View(id)})
}

func (s *Server) handleTabs(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SessionID   string `json:"sessionId"`
		ActiveTabID int    `json:"activeTabId"`
		Tabs        []struct {
			TabID    int        `json:"tabId"`
			WindowID int        `json:"windowId"`
			URL      string     `json:"url"`
			Title    string     `json:"title"`
			Active   bool       `json:"active"`
			Elements *[]Element `json:"elements"`
		} `json:"tabs"`
	}
	if !decode(w, r, &body) {
		return
	}
	id := sessionID(body.SessionID)
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sessionId"})
		return
	}
	reports := make([]TabReport, 0, len(body.Tabs))
	for _, tab := range body.Tabs {
		tab.Elements = trimOptionalElements(tab.Elements)
		reports = append(reports, TabReport{
			TabID:    tab.TabID,
			WindowID: tab.WindowID,
			URL:      tab.URL,
			Title:    tab.Title,
			Active:   tab.Active,
			Elements: tab.Elements,
		})
	}
	tabs := s.store.SyncTabs(id, body.ActiveTabID, reports)
	s.publishSession(id)
	s.pushCommands(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "activeTabId": body.ActiveTabID, "tabs": tabs})
}

func (s *Server) handleAvailability(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SessionID string `json:"sessionId"`
		TabID     int    `json:"tabId"`
		Available bool   `json:"available"`
	}
	if !decode(w, r, &body) {
		return
	}
	id := sessionID(body.SessionID)
	if id == "" || body.TabID == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sessionId and tabId are required"})
		return
	}
	if !s.store.SetAvailable(id, body.TabID, body.Available) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown tab"})
		return
	}
	s.publishSession(id)
	writeJSON(w, http.StatusOK, s.store.ViewTab(id, body.TabID))
}

func (s *Server) handleCommands(w http.ResponseWriter, r *http.Request) {
	id := sessionID(r.URL.Query().Get("sessionId"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sessionId"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"commands": s.store.TakeQueued(id)})
}

func (s *Server) handleResult(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SessionID string `json:"sessionId"`
		CommandID string `json:"commandId"`
		OK        bool   `json:"ok"`
		Detail    string `json:"detail"`
	}
	if !decode(w, r, &body) {
		return
	}
	id := sessionID(body.SessionID)
	if id == "" || body.CommandID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sessionId and commandId are required"})
		return
	}
	command, ok := s.store.Finish(id, body.CommandID, body.Detail, body.OK)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown command"})
		return
	}
	s.publishSession(id)
	writeJSON(w, http.StatusOK, command)
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	id := sessionID(r.PathValue("id"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sessionId"})
		return
	}
	focus := 0
	if raw := r.URL.Query().Get("tabId"); raw != "" {
		focus = atoi(raw)
	}
	writeJSON(w, http.StatusOK, s.store.ViewTab(id, focus))
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SessionID string `json:"sessionId"`
		TabID     int    `json:"tabId"`
		Message   string `json:"message"`
		Confirmed bool   `json:"confirmed"`
	}
	if !decode(w, r, &body) {
		return
	}
	id := sessionID(body.SessionID)
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sessionId"})
		return
	}
	message := strings.TrimSpace(body.Message)
	if body.Confirmed {
		command, ok := s.store.ReleaseHeld(id, body.TabID)
		if !ok {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "nothing is waiting for confirmation"})
			return
		}
		s.store.Append(id, command.TabID, "assistant", "Confirmed. Running "+command.Type+".", "", false)
		s.publishSession(id)
		s.pushCommands(id)
		writeJSON(w, http.StatusOK, map[string]any{
			"reply":             "Confirmed. Running " + command.Type + ".",
			"needsConfirmation": false,
			"command":           command,
			"session":           s.store.ViewTab(id, command.TabID),
		})
		return
	}
	if message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}
	page, history, problem := s.store.Context(id, body.TabID)
	if problem != "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": problem})
		return
	}
	tabID := page.TabID
	s.store.ClearHeld(id, tabID)
	s.store.Append(id, tabID, "user", message, "", false)
	decision, err := s.callAgent(message, page, history)
	if err != nil {
		s.store.Append(id, tabID, "assistant", "The decision service is unavailable.", "", false)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	var command *Command
	action, _ := decision["action"].(map[string]any)
	needsConfirmation, _ := decision["needsConfirmation"].(bool)
	reply, _ := decision["reply"].(string)
	backend, _ := decision["backend"].(string)
	if parsed, ok := parseAction(action); ok {
		if needsConfirmation {
			s.store.Hold(id, tabID, parsed)
		} else if queued, enqueueErr := s.store.Enqueue(id, tabID, parsed); enqueueErr != "" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": enqueueErr})
			return
		} else {
			command = queued
		}
	}
	s.store.Append(id, tabID, "assistant", reply, backend, needsConfirmation)
	s.publishSession(id)
	s.pushCommands(id)
	writeJSON(w, http.StatusOK, map[string]any{
		"reply":             reply,
		"backend":           backend,
		"layaStatus":        decision["layaStatus"],
		"needsConfirmation": needsConfirmation,
		"decision":          decision["decision"],
		"command":           command,
		"session":           s.store.ViewTab(id, tabID),
	})
}

func (s *Server) callAgent(prompt string, page *Page, history []Message) (map[string]any, error) {
	payload := map[string]any{"prompt": prompt, "confirmed": false, "history": history, "page": page}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(s.agentURL, "/")+"/decide", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		if message, _ := decoded["error"].(string); message != "" {
			return nil, &httpError{message}
		}
		return nil, &httpError{strings.TrimSpace(string(body))}
	}
	return decoded, nil
}

type httpError struct{ message string }

func (e *httpError) Error() string { return e.message }

func parseAction(raw map[string]any) (Action, bool) {
	if raw == nil {
		return Action{}, false
	}
	action := Action{
		Type:      stringField(raw, "type"),
		ElementID: stringField(raw, "elementId"),
		Text:      stringField(raw, "text"),
		URL:       stringField(raw, "url"),
	}
	if !allowedActions[action.Type] {
		return Action{}, false
	}
	if action.Type == "navigate" && !strings.HasPrefix(action.URL, "http://") && !strings.HasPrefix(action.URL, "https://") {
		return Action{}, false
	}
	return action, true
}

func stringField(raw map[string]any, key string) string {
	value, _ := raw[key].(string)
	return value
}

func atoi(value string) int {
	n := 0
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

func sessionID(value string) string {
	if value == "" {
		return "local"
	}
	if sessionPattern.MatchString(value) {
		return value
	}
	return ""
}

func decode(w http.ResponseWriter, r *http.Request, dest any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := decoder.Decode(dest); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "encode failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
