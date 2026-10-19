package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

type subscriber struct {
	sessionID string
	role      string
	watch     int
	conn      *websocket.Conn
	mu        sync.Mutex
}

func (sub *subscriber) write(payload any) error {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	_ = sub.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return sub.conn.WriteJSON(payload)
}

type Hub struct {
	mu   sync.Mutex
	subs map[string]map[*subscriber]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: map[string]map[*subscriber]struct{}{}}
}

func (h *Hub) join(sessionID, role string, watch int, conn *websocket.Conn) *subscriber {
	sub := &subscriber{sessionID: sessionID, role: role, watch: watch, conn: conn}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[sessionID] == nil {
		h.subs[sessionID] = map[*subscriber]struct{}{}
	}
	h.subs[sessionID][sub] = struct{}{}
	return sub
}

func (h *Hub) leave(sub *subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set := h.subs[sub.sessionID]; set != nil {
		delete(set, sub)
		if len(set) == 0 {
			delete(h.subs, sub.sessionID)
		}
	}
	_ = sub.conn.Close()
}

func (h *Hub) each(sessionID string, fn func(*subscriber)) {
	h.mu.Lock()
	list := make([]*subscriber, 0, len(h.subs[sessionID]))
	for sub := range h.subs[sessionID] {
		list = append(list, sub)
	}
	h.mu.Unlock()
	for _, sub := range list {
		fn(sub)
	}
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	id := sessionID(r.URL.Query().Get("sessionId"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sessionId"})
		return
	}
	role := r.URL.Query().Get("role")
	if role != "extension" {
		role = "ui"
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	sub := s.hub.join(id, role, atoi(r.URL.Query().Get("tabId")), conn)
	defer s.hub.leave(sub)
	s.writeSession(sub)
	if role == "extension" {
		s.pushCommands(id)
	}
	conn.SetReadLimit(1 << 20)
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var msg struct {
			Type       string `json:"type"`
			TabID      int    `json:"tabId"`
			ActiveTab  int    `json:"activeTabId"`
			CommandID  string `json:"commandId"`
			OK         bool   `json:"ok"`
			Detail     string `json:"detail"`
			Tabs       []struct {
				TabID    int        `json:"tabId"`
				WindowID int        `json:"windowId"`
				URL      string     `json:"url"`
				Title    string     `json:"title"`
				Active   bool       `json:"active"`
				Elements *[]Element `json:"elements"`
			} `json:"tabs"`
		}
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		switch msg.Type {
		case "watch":
			sub.watch = msg.TabID
			s.writeSession(sub)
		case "tabs":
			reports := make([]TabReport, 0, len(msg.Tabs))
			for _, tab := range msg.Tabs {
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
			s.store.SyncTabs(id, msg.ActiveTab, reports)
			s.publishSession(id)
			for _, tab := range msg.Tabs {
				s.noteURL(id, tab.TabID, tab.URL)
			}
		case "result":
			if msg.CommandID == "" {
				continue
			}
			if command, ok := s.store.Finish(id, msg.CommandID, msg.Detail, msg.OK); ok {
				s.publishSession(id)
				s.afterCommand(id, command)
			}
		}
	}
}

func (s *Server) writeSession(sub *subscriber) {
	_ = sub.write(map[string]any{"type": "session", "session": s.store.ViewTab(sub.sessionID, sub.watch)})
}

func (s *Server) publishSession(id string) {
	if s.hub == nil {
		return
	}
	s.hub.each(id, func(sub *subscriber) {
		if err := sub.write(map[string]any{"type": "session", "session": s.store.ViewTab(id, sub.watch)}); err != nil {
			s.hub.leave(sub)
		}
	})
}

func (s *Server) pushCommands(id string) {
	if s.hub == nil {
		return
	}
	commands := s.store.PeekQueued(id)
	if len(commands) == 0 {
		return
	}
	sent := false
	s.hub.each(id, func(sub *subscriber) {
		if sub.role != "extension" {
			return
		}
		if err := sub.write(map[string]any{"type": "commands", "commands": commands}); err != nil {
			s.hub.leave(sub)
			return
		}
		sent = true
	})
	if sent {
		s.store.TakeQueued(id)
	}
}
