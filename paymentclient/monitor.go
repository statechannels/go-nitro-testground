package paymentclient

import (
	"time"

	nitroclient "github.com/statechannels/go-nitro/client"
	"github.com/statechannels/go-nitro/client/engine/store/safesync"
	"github.com/statechannels/go-nitro/protocols"
)

const SLEEP_TIME = time.Microsecond * 500

// CompletionMonitor is a struct used to watch for objective completion
type completionMonitor struct {
	completed *safesync.Map[bool]
	client    *nitroclient.Client
	quit      chan struct{}
}

// NewCompletionMonitor creates a new completion monitor
func newCompletionMonitor(client *nitroclient.Client) *completionMonitor {

	completed := safesync.Map[bool]{}

	c := &completionMonitor{
		completed: &completed,
		client:    client,
		quit:      make(chan struct{}),
	}
	go c.watch()
	return c
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

// waitForObjectivesToComplete blocks until all objectives are completed
func (c *completionMonitor) waitForObjectivesToComplete(ids []protocols.ObjectiveId) {
	for {

		if c.done(ids) {

			break
		}
		time.Sleep(SLEEP_TIME)
	}
}

// close stops the completion monitor from listening to the CompletedObjectives chan
func (c *completionMonitor) close() {
	close(c.quit)
}
