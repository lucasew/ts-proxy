package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				err := sm.Transition(to)
				if !assert.NoError(t, err, "transition %d (%s -> %s)", i, sm.Current(), to) {
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
			assert.Error(t, err, "expected error for %s -> %s", tt.from, tt.to)
		})
	}
}

func TestStateMachineReset(t *testing.T) {
	sm := NewStateMachine()
	require.NoError(t, sm.Transition(StateStarting))
	sm.Reset()
	require.Equal(t, StateInit, sm.Current())
	require.NoError(t, sm.Transition(StateStarting))
}

func TestStateMachineConcurrent(t *testing.T) {
	sm := NewStateMachine()
	require.NoError(t, sm.Transition(StateStarting))
	require.NoError(t, sm.Transition(StateRunning))

	done := make(chan State, 100)
	for range 100 {
		go func() {
			done <- sm.Current()
		}()
	}
	for range 100 {
		assert.Equal(t, StateRunning, <-done)
	}
}
