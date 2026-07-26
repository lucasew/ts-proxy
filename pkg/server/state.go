package server

import (
	"fmt"
	"sync"
)

// State represents a server lifecycle state.
type State string

const (
	StateInit           State = "init"
	StateStarting       State = "starting"
	StateAuthenticating State = "authenticating"
	StateRunning        State = "running"
	StateFailed         State = "failed"
	StateStopped        State = "stopped"
)

// stateSentinel is a stable state-machine error identity for errors.Is / %w.
type stateSentinel string

func (e stateSentinel) Error() string { return string(e) }

// ErrInvalidStateTransition is the root cause when Transition rejects a move.
// Call sites wrap it with fmt.Errorf %w so callers can errors.Is against it.
const ErrInvalidStateTransition stateSentinel = "invalid state transition"

var validTransitions = map[State][]State{
	StateInit:           {StateStarting},
	StateStarting:       {StateAuthenticating, StateRunning, StateFailed},
	StateAuthenticating: {StateRunning, StateFailed},
	StateRunning:        {StateFailed, StateStopped},
	StateFailed:         {StateStarting, StateStopped},
	StateStopped:        {StateStarting},
}

// StateMachine tracks server lifecycle state with enforced transitions.
type StateMachine struct {
	state State
	mu    sync.RWMutex
}

// NewStateMachine creates a state machine in the Init state.
func NewStateMachine() *StateMachine {
	return &StateMachine{state: StateInit}
}

// Current returns the current state.
func (sm *StateMachine) Current() State {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state
}

// Transition moves to a new state if the transition is valid.
func (sm *StateMachine) Transition(to State) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if !isValidTransition(sm.state, to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidStateTransition, sm.state, to)
	}
	sm.state = to
	return nil
}

func isValidTransition(from, to State) bool {
	for _, valid := range validTransitions[from] {
		if valid == to {
			return true
		}
	}
	return false
}
