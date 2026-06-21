package server

import (
	"testing"
)

func TestStateMachineValidTransitions(t *testing.T) {
	tests := []struct {
		name        string
		transitions []State
		wantErr     bool
	}{
		{
			name:        "normal lifecycle",
			transitions: []State{StateStarting, StateAuthenticating, StateRunning, StateStopped},
		},
		{
			name:        "start directly to running",
			transitions: []State{StateStarting, StateRunning, StateStopped},
		},
		{
			name:        "failure and restart",
			transitions: []State{StateStarting, StateAuthenticating, StateFailed, StateStarting, StateRunning},
		},
		{
			name:        "start failure",
			transitions: []State{StateStarting, StateFailed, StateStarting, StateAuthenticating, StateRunning},
		},
		{
			name:        "running to failed",
			transitions: []State{StateStarting, StateRunning, StateFailed},
		},
		{
			name:        "stopped to starting (restart)",
			transitions: []State{StateStarting, StateRunning, StateStopped, StateStarting},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := NewStateMachine()
			for i, to := range tt.transitions {
				if err := sm.Transition(to); err != nil {
					t.Errorf("transition %d (%s -> %s): %v", i, sm.Current(), to, err)
					return
				}
			}
		})
	}
}

func TestStateMachineInvalidTransitions(t *testing.T) {
	tests := []struct {
		name string
		from State
		to   State
	}{
		{"init to running", StateInit, StateRunning},
		{"init to failed", StateInit, StateFailed},
		{"init to stopped", StateInit, StateStopped},
		{"init to authenticating", StateInit, StateAuthenticating},
		{"running to starting", StateRunning, StateStarting},
		{"running to authenticating", StateRunning, StateAuthenticating},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := NewStateMachine()
			// Get to the 'from' state
			switch tt.from {
			case StateStarting:
				sm.Transition(StateStarting)
			case StateRunning:
				sm.Transition(StateStarting)
				sm.Transition(StateRunning)
			case StateFailed:
				sm.Transition(StateStarting)
				sm.Transition(StateFailed)
			case StateStopped:
				sm.Transition(StateStarting)
				sm.Transition(StateRunning)
				sm.Transition(StateStopped)
			}

			if sm.Current() != tt.from {
				t.Skipf("could not reach state %s", tt.from)
			}

			err := sm.Transition(tt.to)
			if err == nil {
				t.Errorf("expected error for %s -> %s, got nil", tt.from, tt.to)
			}
		})
	}
}

func TestStateMachineConcurrent(t *testing.T) {
	sm := NewStateMachine()
	sm.Transition(StateStarting)
	sm.Transition(StateRunning)

	done := make(chan struct{})
	for i := 0; i < 100; i++ {
		go func() {
			_ = sm.Current()
			done <- struct{}{}
		}()
	}
	for i := 0; i < 100; i++ {
		<-done
	}
}
