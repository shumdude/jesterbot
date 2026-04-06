package domain

import "errors"

var (
	ErrNotFound          = errors.New("not found")
	ErrAlreadyRegistered = errors.New("user already registered")
	ErrInvalidUTCOffset  = errors.New("invalid utc offset")
	ErrInvalidClock      = errors.New("invalid clock")
	ErrNoActivities      = errors.New("no activities configured")
	ErrPlanNotReady      = errors.New("day plan is not ready")
	ErrPlanClosed        = errors.New("day plan is closed")
	ErrEmptyTitle        = errors.New("title is required")
)
