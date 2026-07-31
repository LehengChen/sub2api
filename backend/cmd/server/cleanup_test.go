package main

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunCleanupPhasesPreservesWriteAndInfrastructureOrdering(t *testing.T) {
	var mu sync.Mutex
	events := make([]string, 0, 6)
	appendEvent := func(event string) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	}

	ordered := []cleanupStep{
		{name: "usage", fn: func() error { appendEvent("usage"); return errors.New("reported only") }},
		{name: "billing", fn: func() error { appendEvent("billing"); return nil }},
		{name: "quota", fn: func() error { appendEvent("quota"); return nil }},
	}
	parallel := []cleanupStep{
		{name: "background-a", fn: func() error { appendEvent("background-a"); return nil }},
		{name: "background-b", fn: func() error { appendEvent("background-b"); return nil }},
	}
	infrastructure := []cleanupStep{
		{name: "redis", fn: func() error { appendEvent("redis"); return nil }},
	}

	runCleanupPhases(ordered, parallel, infrastructure)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"usage", "billing", "quota"}, events[:3])
	require.ElementsMatch(t, []string{"background-a", "background-b"}, events[3:5])
	require.Equal(t, "redis", events[5])
}
