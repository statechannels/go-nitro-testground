package main

import (
	"time"

	nitroclient "github.com/statechannels/go-nitro/client"
	"github.com/statechannels/go-nitro/protocols"
)

const SLEEP_TIME = time.Millisecond * 10

// completionMonitor is a struct used to watch for objective completion
type completionMonitor struct {
	completed map[protocols.ObjectiveId]bool
	client    *nitroclient.Client
	quit      chan struct{}
}

// NewCompletionMonitor creates a new completion monitor
func NewCompletionMonitor(client *nitroclient.Client) *completionMonitor {

	completed := make(map[protocols.ObjectiveId]bool)

	c := &completionMonitor{
		completed: completed,
		client:    client,
		quit:      make(chan struct{}),
	}
	go c.watch()
	return c
}

// Add adds the objective id for us to monitor
func (c *completionMonitor) Add(id protocols.ObjectiveId) {
	c.completed[id] = false
}

func (c *completionMonitor) AllDone() bool {
	for _, isComplete := range c.completed {
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

			c.completed[id] = true

		case <-c.quit:
			return
		}
	}
}

// Wait blocks until all objectives are completed
func (c *completionMonitor) Wait() {
	for {
		time.Sleep(SLEEP_TIME)

		if c.AllDone() {
			break
		}
	}
}

// Stop stops the completion monitor listening to the CompletedObjectives chan
func (c *completionMonitor) Stop() {
	close(c.quit)
}
