package main

import (
	"log"
	"sync"
)

type cleanupStep struct {
	name string
	fn   func() error
}

// runCleanupPhases preserves dependencies between shutdown writes: ordered
// producers/consumers drain first, independent services then stop in parallel,
// and shared infrastructure closes only after every application step returns.
func runCleanupPhases(ordered, parallel, infrastructure []cleanupStep) {
	runCleanupSequential(ordered)
	runCleanupParallel(parallel)
	runCleanupSequential(infrastructure)
}

func runCleanupParallel(steps []cleanupStep) {
	var wg sync.WaitGroup
	for i := range steps {
		step := steps[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			runCleanupStep(step)
		}()
	}
	wg.Wait()
}

func runCleanupSequential(steps []cleanupStep) {
	for i := range steps {
		runCleanupStep(steps[i])
	}
}

func runCleanupStep(step cleanupStep) {
	if err := step.fn(); err != nil {
		log.Printf("[Cleanup] %s failed: %v", step.name, err)
		return
	}
	log.Printf("[Cleanup] %s succeeded", step.name)
}
