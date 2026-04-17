package session_backend

import "github.com/go-telegram/fsm"

func New(initialScene string) *fsm.FSM[string, map[string]string] {
	return fsm.New[string, map[string]string](fsm.StateID(initialScene), nil)
}
