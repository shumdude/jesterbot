package telegram

import (
	"sync"

	"jesterbot/internal/domain"
)

type SessionState string

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
)

type Session struct {
	State              SessionState
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

func (s *SessionStore) Get(chatID int64) Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.items[chatID]
	if !ok {
		return Session{State: stateIdle}
	}

	// Return a value copy so callers cannot mutate shared session state
	// without going through Update (which provides synchronization).
	return *session
}

func (s *SessionStore) Update(chatID int64, update func(*Session)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.items[chatID]
	if !ok {
		session = &Session{State: stateIdle}
		s.items[chatID] = session
	}

	update(session)
}

func (s *SessionStore) Clear(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, chatID)
}
