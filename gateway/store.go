package main

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type Element struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Name      string `json:"name"`
	Tag       string `json:"tag"`
	Editable  bool   `json:"editable"`
	InputType string `json:"inputType"`
	Value     string `json:"value"`
	Href      string `json:"href"`
}

type Page struct {
	URL        string    `json:"url"`
	Title      string    `json:"title"`
	Elements   []Element `json:"elements"`
	TabID      int       `json:"tabId"`
	ReportedAt string    `json:"reportedAt"`
}

type Action struct {
	Type      string `json:"type"`
	ElementID string `json:"elementId"`
	Text      string `json:"text"`
	URL       string `json:"url"`
}

type Command struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	TabID     int    `json:"tabId"`
	Type      string `json:"type"`
	ElementID string `json:"elementId"`
	Text      string `json:"text"`
	URL       string `json:"url"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
	OK        *bool  `json:"ok,omitempty"`
}

type Message struct {
	Role              string `json:"role"`
	Content           string `json:"content"`
	At                string `json:"at"`
	NeedsConfirmation bool   `json:"needsConfirmation,omitempty"`
	Backend           string `json:"backend,omitempty"`
}

type session struct {
	ID         string
	Messages   []Message
	Page       *Page
	Held       *Action
	HeldReply  string
	Commands   []*Command
	LastResult *Command
}

type SessionView struct {
	SessionID          string    `json:"sessionId"`
	Messages           []Message `json:"messages"`
	Page               *Page     `json:"page,omitempty"`
	ExtensionConnected bool      `json:"extensionConnected"`
	ExtensionLastSeen  string    `json:"extensionLastSeen,omitempty"`
	Held               *Action   `json:"held,omitempty"`
	LastResult         *Command  `json:"lastResult,omitempty"`
}

type Store struct {
	mu       sync.Mutex
	sessions map[string]*session
}

func NewStore() *Store {
	return &Store{sessions: map[string]*session{}}
}

func (s *Store) get(id string) *session {
	sess, ok := s.sessions[id]
	if !ok {
		sess = &session{ID: id}
		s.sessions[id] = sess
	}
	return sess
}

func (s *Store) SavePage(id string, page Page) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.get(id)
	page.ReportedAt = time.Now().UTC().Format(time.RFC3339)
	sess.Page = &page
}

func (s *Store) Append(id, role, content, backend string, needsConfirmation bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.get(id)
	sess.Messages = append(sess.Messages, Message{
		Role:              role,
		Content:           content,
		At:                time.Now().UTC().Format(time.RFC3339),
		NeedsConfirmation: needsConfirmation,
		Backend:           backend,
	})
	if len(sess.Messages) > 200 {
		sess.Messages = sess.Messages[len(sess.Messages)-200:]
	}
}

func (s *Store) Context(id string) (*Page, []Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.get(id)
	history := append([]Message(nil), sess.Messages...)
	if sess.Page == nil {
		return nil, history
	}
	page := *sess.Page
	page.Elements = append([]Element(nil), sess.Page.Elements...)
	return &page, history
}

func (s *Store) Hold(id string, action Action) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.get(id)
	sess.Held = &action
}

func (s *Store) ClearHeld(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.get(id).Held = nil
}

func (s *Store) Enqueue(id string, action Action) *Command {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.get(id)
	command := &Command{
		ID:        newID(),
		SessionID: id,
		Type:      action.Type,
		ElementID: action.ElementID,
		Text:      action.Text,
		URL:       action.URL,
		Status:    "queued",
	}
	if sess.Page != nil {
		command.TabID = sess.Page.TabID
	}
	sess.Held = nil
	sess.Commands = append(sess.Commands, command)
	return command
}

func (s *Store) ReleaseHeld(id string) (*Command, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.get(id)
	if sess.Held == nil {
		return nil, false
	}
	action := *sess.Held
	sess.Held = nil
	command := &Command{
		ID:        newID(),
		SessionID: id,
		Type:      action.Type,
		ElementID: action.ElementID,
		Text:      action.Text,
		URL:       action.URL,
		Status:    "queued",
	}
	if sess.Page != nil {
		command.TabID = sess.Page.TabID
	}
	sess.Commands = append(sess.Commands, command)
	return command, true
}

func (s *Store) TakeQueued(id string) []Command {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return []Command{}
	}
	out := []Command{}
	for _, command := range sess.Commands {
		if command.Status == "queued" {
			command.Status = "delivered"
			out = append(out, *command)
		}
	}
	return out
}

func (s *Store) Finish(id, commandID, detail string, ok bool) (*Command, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, exists := s.sessions[id]
	if !exists {
		return nil, false
	}
	for _, command := range sess.Commands {
		if command.ID != commandID {
			continue
		}
		command.Detail = detail
		command.OK = &ok
		if ok {
			command.Status = "done"
		} else {
			command.Status = "error"
		}
		copied := *command
		sess.LastResult = &copied
		return &copied, true
	}
	return nil, false
}

func (s *Store) View(id string) SessionView {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return SessionView{SessionID: id, Messages: []Message{}}
	}
	view := SessionView{
		SessionID:  id,
		Messages:   append([]Message(nil), sess.Messages...),
		LastResult: sess.LastResult,
		Held:       sess.Held,
	}
	if sess.Page != nil {
		page := *sess.Page
		page.Elements = append([]Element(nil), sess.Page.Elements...)
		view.Page = &page
		view.ExtensionLastSeen = page.ReportedAt
		if reported, err := time.Parse(time.RFC3339, page.ReportedAt); err == nil {
			view.ExtensionConnected = time.Since(reported) < 8*time.Second
		}
	}
	if view.Messages == nil {
		view.Messages = []Message{}
	}
	return view
}

func (s *Store) SessionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("150405.000")))
	}
	return hex.EncodeToString(buf)
}
