package telegram

import "sync"

type SessionState string

const (
	stateIdle            SessionState = "idle"
	stateRegisterName    SessionState = "register_name"
	stateRegisterOffset  SessionState = "register_offset"
	stateRegisterMorning SessionState = "register_morning"
	stateAddActivity     SessionState = "add_activity"
	stateEditActivity    SessionState = "edit_activity"
	stateUpdateMorning   SessionState = "update_morning"
	stateUpdateReminder  SessionState = "update_reminder"
)

type Session struct {
	State          SessionState
	Name           string
	UTCOffset      string
	EditActivityID int64
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
