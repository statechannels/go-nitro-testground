package main

import (
	"time"

	nitroclient "github.com/statechannels/go-nitro/client"
	"github.com/statechannels/go-nitro/client/engine/store/safesync"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/testground/sdk-go/runtime"
)

const SLEEP_TIME = time.Microsecond * 500

// completionMonitor is a struct used to watch for objective completion
type completionMonitor struct {
	completed *safesync.Map[bool]
	client    *nitroclient.Client
	quit      chan struct{}
	runenv    *runtime.RunEnv
}

// NewCompletionMonitor creates a new completion monitor
func NewCompletionMonitor(client *nitroclient.Client, runenv *runtime.RunEnv) *completionMonitor {

	completed := safesync.Map[bool]{}

	c := &completionMonitor{
		completed: &completed,
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

// checks whether the given objectives are complete
func (c *completionMonitor) done(ids []protocols.ObjectiveId) bool {

	for _, id := range ids {
		isComplete, _ := c.completed.Load(string(id))
		if !isComplete {
			return false
		}
	}
	return true

}

// watch runs in a gofunc and listens to the CompletedObjectives chan
func (c *completionMonitor) watch() {
	for {
		select {
		case id := <-c.client.CompletedObjectives():
			c.completed.Store(string(id), true)
		case <-c.quit:
			return
		}
	}
}

// WaitForObjectivesToComplete blocks until all objectives are completed
func (c *completionMonitor) WaitForObjectivesToComplete(ids []protocols.ObjectiveId) {
	for {

		if c.done(ids) {

			break
		}
		time.Sleep(SLEEP_TIME)
	}
}

// Close stops the completion monitor from listening to the CompletedObjectives chan
func (c *completionMonitor) Close() {
	close(c.quit)
}
