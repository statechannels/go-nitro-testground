package main

import (
	"time"

	nitroclient "github.com/statechannels/go-nitro/client"
	"github.com/statechannels/go-nitro/client/engine/store/safesync"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/testground/sdk-go/runtime"
)

const SLEEP_TIME = time.Millisecond * 10

// completionMonitor is a struct used to watch for objective completion
type completionMonitor struct {
	completed safesync.Map[bool]
	client    *nitroclient.Client
	quit      chan struct{}
	runenv    runtime.RunEnv
}

// NewCompletionMonitor creates a new completion monitor
func NewCompletionMonitor(client *nitroclient.Client, runenv runtime.RunEnv) *completionMonitor {

	completed := safesync.Map[bool]{}

	c := &completionMonitor{
		completed: completed,
		client:    client,
		quit:      make(chan struct{}),
		runenv:    runenv,
	}
	go c.watch()
	return c
}

// WatchObjective adds the objective id to the list of objectives to watch
func (c *completionMonitor) WatchObjective(id protocols.ObjectiveId) {
	c.completed.Store(string(id), false)
}

func (c *completionMonitor) done() bool {
	allComplete := true
	c.completed.Range(func(id string, isComplete bool) bool {
		if !isComplete {
			allComplete = false
			return false
		}

		return true
	})
	return allComplete
}

// watch runs in a gofunc and listens to the CompletedObjectives chan
func (c *completionMonitor) watch() {
	for {
		select {
		case id := <-c.client.CompletedObjectives():
			c.runenv.D().Counter("completed-objectives").Inc(1)
			c.runenv.RecordMessage("objective complete %s", id)
			c.completed.Store(string(id), true)

		case <-c.quit:
			return
		}
	}
}

// WaitForObjectivesToComplete blocks until all objectives are completed
func (c *completionMonitor) WaitForObjectivesToComplete() {
	for {

		if c.done() {
			close(c.quit)
			break
		}
		time.Sleep(SLEEP_TIME)
	}
}
