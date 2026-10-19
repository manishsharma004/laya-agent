package main

import (
	"crypto/rand"
	"encoding/hex"
	"sort"
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

type TabState struct {
	TabID      int
	WindowID   int
	URL        string
	Title      string
	Active     bool
	Available  bool
	Elements   []Element
	ReportedAt string
	Messages   []Message
	Held       *Action
	LastResult *Command
}

type TabReport struct {
	TabID    int
	WindowID int
	URL      string
	Title    string
	Active   bool
	Elements *[]Element
}

type TabSummary struct {
	TabID        int    `json:"tabId"`
	WindowID     int    `json:"windowId"`
	URL          string `json:"url"`
	Title        string `json:"title"`
	Active       bool   `json:"active"`
	Available    bool   `json:"available"`
	ElementCount int    `json:"elementCount"`
	ReportedAt   string `json:"reportedAt"`
}

type session struct {
	ID          string
	Tabs        map[int]*TabState
	ActiveTabID int
	Commands    []*Command
}

type SessionView struct {
	SessionID          string       `json:"sessionId"`
	ActiveTabID        int          `json:"activeTabId"`
	FocusTabID         int          `json:"focusTabId"`
	Tabs               []TabSummary `json:"tabs"`
	Messages           []Message    `json:"messages"`
	Page               *Page        `json:"page,omitempty"`
	ExtensionConnected bool         `json:"extensionConnected"`
	ExtensionLastSeen  string       `json:"extensionLastSeen,omitempty"`
	Held               *Action      `json:"held,omitempty"`
	LastResult         *Command     `json:"lastResult,omitempty"`
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
		sess = &session{ID: id, Tabs: map[int]*TabState{}}
		s.sessions[id] = sess
	}
	if sess.Tabs == nil {
		sess.Tabs = map[int]*TabState{}
	}
	return sess
}

func (s *Store) SavePage(id string, page Page) {
	if page.TabID == 0 {
		return
	}
	elements := append([]Element(nil), page.Elements...)
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.get(id)
	tab, ok := sess.Tabs[page.TabID]
	if !ok {
		tab = &TabState{TabID: page.TabID, Available: true}
		sess.Tabs[page.TabID] = tab
	}
	tab.URL = page.URL
	tab.Title = page.Title
	tab.Elements = elements
	tab.ReportedAt = time.Now().UTC().Format(time.RFC3339)
	if sess.ActiveTabID == 0 {
		sess.ActiveTabID = page.TabID
		tab.Active = true
	}
}

func (s *Store) SyncTabs(id string, activeTabID int, reports []TabReport) []TabSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.get(id)
	seen := map[int]bool{}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, report := range reports {
		if report.TabID == 0 {
			continue
		}
		seen[report.TabID] = true
		tab, ok := sess.Tabs[report.TabID]
		if !ok {
			tab = &TabState{TabID: report.TabID, Available: true}
			sess.Tabs[report.TabID] = tab
		}
		tab.WindowID = report.WindowID
		tab.URL = report.URL
		tab.Title = report.Title
		tab.Active = report.Active
		tab.ReportedAt = now
		if report.Elements != nil {
			tab.Elements = append([]Element(nil), (*report.Elements)...)
		}
	}
	for tabID := range sess.Tabs {
		if !seen[tabID] {
			delete(sess.Tabs, tabID)
		}
	}
	if _, ok := sess.Tabs[activeTabID]; ok {
		sess.ActiveTabID = activeTabID
	} else {
		sess.ActiveTabID = 0
		for _, tab := range sess.Tabs {
			if tab.Active {
				sess.ActiveTabID = tab.TabID
				break
			}
		}
	}
	for _, tab := range sess.Tabs {
		tab.Active = tab.TabID == sess.ActiveTabID
	}
	return summarizeLocked(sess)
}

func (s *Store) SetAvailable(id string, tabID int, available bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return false
	}
	tab, ok := sess.Tabs[tabID]
	if !ok {
		return false
	}
	tab.Available = available
	if !available {
		tab.Held = nil
	}
	return true
}

func (s *Store) Append(id string, tabID int, role, content, backend string, needsConfirmation bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tab := s.tabLocked(id, tabID)
	if tab == nil {
		return
	}
	tab.Messages = append(tab.Messages, Message{
		Role:              role,
		Content:           content,
		At:                time.Now().UTC().Format(time.RFC3339),
		NeedsConfirmation: needsConfirmation,
		Backend:           backend,
	})
	if len(tab.Messages) > 200 {
		tab.Messages = tab.Messages[len(tab.Messages)-200:]
	}
}

func (s *Store) Context(id string, tabID int) (*Page, []Message, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.get(id)
	tabID = resolveTabLocked(sess, tabID)
	tab := sess.Tabs[tabID]
	if tab == nil {
		return nil, nil, "no open tab"
	}
	if !tab.Available {
		return nil, nil, "tab is not available"
	}
	history := append([]Message(nil), tab.Messages...)
	page := pageFrom(tab)
	return &page, history, ""
}

func (s *Store) Hold(id string, tabID int, action Action) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tab := s.tabLocked(id, tabID); tab != nil {
		tab.Held = &action
	}
}

func (s *Store) ClearHeld(id string, tabID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.get(id)
	tabID = resolveTabLocked(sess, tabID)
	if tab := sess.Tabs[tabID]; tab != nil {
		tab.Held = nil
	}
}

func (s *Store) Enqueue(id string, tabID int, action Action) (*Command, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.get(id)
	tabID = resolveTabLocked(sess, tabID)
	tab := sess.Tabs[tabID]
	if tab == nil {
		return nil, "no open tab"
	}
	if !tab.Available {
		return nil, "tab is not available"
	}
	command := newCommand(id, tabID, action)
	tab.Held = nil
	sess.Commands = append(sess.Commands, command)
	return command, ""
}

func (s *Store) ReleaseHeld(id string, tabID int) (*Command, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.get(id)
	tabID = resolveTabLocked(sess, tabID)
	tab := sess.Tabs[tabID]
	if tab == nil || tab.Held == nil || !tab.Available {
		return nil, false
	}
	action := *tab.Held
	tab.Held = nil
	command := newCommand(id, tabID, action)
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
		tab := sess.Tabs[command.TabID]
		if command.Status == "queued" && tab != nil && tab.Available {
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
		if tab := sess.Tabs[command.TabID]; tab != nil {
			tab.LastResult = &copied
			tab.Messages = append(tab.Messages, Message{
				Role:    "system",
				Content: detail,
				At:      time.Now().UTC().Format(time.RFC3339),
			})
		}
		return &copied, true
	}
	return nil, false
}

func (s *Store) View(id string) SessionView {
	return s.ViewTab(id, 0)
}

func (s *Store) ViewTab(id string, focus int) SessionView {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return SessionView{SessionID: id, Messages: []Message{}, Tabs: []TabSummary{}}
	}
	focus = resolveTabLocked(sess, focus)
	view := SessionView{
		SessionID:   id,
		ActiveTabID: sess.ActiveTabID,
		FocusTabID:  focus,
		Tabs:        summarizeLocked(sess),
		Messages:    []Message{},
	}
	tab := sess.Tabs[focus]
	if tab == nil {
		return view
	}
	view.Messages = append([]Message(nil), tab.Messages...)
	page := pageFrom(tab)
	view.Page = &page
	view.Held = tab.Held
	view.LastResult = tab.LastResult
	view.ExtensionLastSeen = tab.ReportedAt
	if reported, err := time.Parse(time.RFC3339, tab.ReportedAt); err == nil {
		view.ExtensionConnected = time.Since(reported) < 8*time.Second
	}
	return view
}

func (s *Store) SessionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

func (s *Store) tabLocked(id string, tabID int) *TabState {
	sess := s.get(id)
	tabID = resolveTabLocked(sess, tabID)
	return sess.Tabs[tabID]
}

func resolveTabLocked(sess *session, tabID int) int {
	if tabID != 0 {
		if _, ok := sess.Tabs[tabID]; ok {
			return tabID
		}
	}
	if _, ok := sess.Tabs[sess.ActiveTabID]; ok {
		return sess.ActiveTabID
	}
	return tabID
}

func pageFrom(tab *TabState) Page {
	return Page{
		URL:        tab.URL,
		Title:      tab.Title,
		Elements:   append([]Element(nil), tab.Elements...),
		TabID:      tab.TabID,
		ReportedAt: tab.ReportedAt,
	}
}

func summarizeLocked(sess *session) []TabSummary {
	out := make([]TabSummary, 0, len(sess.Tabs))
	for _, tab := range sess.Tabs {
		out = append(out, TabSummary{
			TabID:        tab.TabID,
			WindowID:     tab.WindowID,
			URL:          tab.URL,
			Title:        tab.Title,
			Active:       tab.Active,
			Available:    tab.Available,
			ElementCount: len(tab.Elements),
			ReportedAt:   tab.ReportedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Active != out[j].Active {
			return out[i].Active
		}
		return out[i].TabID < out[j].TabID
	})
	return out
}

func newCommand(sessionID string, tabID int, action Action) *Command {
	return &Command{
		ID:        newID(),
		SessionID: sessionID,
		TabID:     tabID,
		Type:      action.Type,
		ElementID: action.ElementID,
		Text:      action.Text,
		URL:       action.URL,
		Status:    "queued",
	}
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("150405.000")))
	}
	return hex.EncodeToString(buf)
}
