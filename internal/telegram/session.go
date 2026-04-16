package telegram

import (
	"sync"

	"jesterbot/internal/domain"
)

type SessionState string
type UIMessageMode string

const (
	// Registration flow states.
	stateIdle            SessionState = "idle"
	stateRegisterName    SessionState = "register_name"
	stateRegisterOffset  SessionState = "register_offset"
	stateRegisterMorning SessionState = "register_morning"
	// Activity management states.
	stateAddActivity    SessionState = "add_activity"
	stateEditActivity   SessionState = "edit_activity"
	stateAddOneOffTitle SessionState = "add_one_off_title"
	stateAddOneOffItems SessionState = "add_one_off_items"
	// Settings update states.
	stateUpdateMorning        SessionState = "update_morning"
	stateUpdateReminder       SessionState = "update_reminder"
	stateUpdateOneOffReminder SessionState = "update_one_off_reminder"
	stateUpdateTickInterval   SessionState = "update_tick_interval"

	uiMessageModeNormal UIMessageMode = "normal"
	uiMessageModeEdit   UIMessageMode = "edit"
	uiMessageModeDelete UIMessageMode = "delete"
)

type Session struct {
	State              SessionState
	MessageMode        UIMessageMode
	ActiveMessageID    int
	Name               string
	UTCOffset          string
	EditActivityID     int64
	OneOffTaskTitle    string
	OneOffTaskPriority domain.OneOffTaskPriority
}

type SessionStore struct {
	mu    sync.Mutex
	items map[int64]*Session
}

func NewSessionStore() *SessionStore {
	return &SessionStore{items: make(map[int64]*Session)}
}

func (s Session) messageMode() UIMessageMode {
	if s.MessageMode == "" {
		return uiMessageModeDelete
	}
	return s.MessageMode
}

func (s *Session) resetForState(state SessionState) {
	messageMode := s.messageMode()
	activeMessageID := s.ActiveMessageID
	*s = Session{
		State:           state,
		MessageMode:     messageMode,
		ActiveMessageID: activeMessageID,
	}
}

func (s *SessionStore) Get(chatID int64) Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.items[chatID]
	if !ok {
		return Session{State: stateIdle, MessageMode: uiMessageModeDelete}
	}

	// Return a value copy so callers cannot mutate shared session state
	// without going through Update (which provides synchronization).
	copy := *session
	copy.MessageMode = copy.messageMode()
	return copy
}

func (s *SessionStore) Update(chatID int64, update func(*Session)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.items[chatID]
	if !ok {
		session = &Session{State: stateIdle, MessageMode: uiMessageModeDelete}
		s.items[chatID] = session
	}
	if session.MessageMode == "" {
		session.MessageMode = uiMessageModeDelete
	}

	update(session)
}

func (s *SessionStore) Clear(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.items[chatID]
	if !ok {
		return
	}
	messageMode := session.messageMode()
	activeMessageID := session.ActiveMessageID
	*session = Session{
		State:           stateIdle,
		MessageMode:     messageMode,
		ActiveMessageID: activeMessageID,
	}
}
