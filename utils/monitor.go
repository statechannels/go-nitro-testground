package utils

import (
	"sync"
	"time"

	nitroclient "github.com/statechannels/go-nitro/client"
	"github.com/statechannels/go-nitro/protocols"
)

const SLEEP_TIME = time.Microsecond * 500

// CompletionMonitor is a struct used to watch for objective completion
type CompletionMonitor struct {
	completed *sync.Map
	client    *nitroclient.Client
	quit      chan struct{}
	log       func(msg string, a ...interface{})
}

// NewCompletionMonitor creates a new completion monitor
func NewCompletionMonitor(client *nitroclient.Client, logFunc func(msg string, a ...interface{})) *CompletionMonitor {

	completed := sync.Map{}

	c := &CompletionMonitor{
		completed: &completed,
		client:    client,
		quit:      make(chan struct{}),
		log:       logFunc,
	}
	go c.watch()
	return c
}

// checks whether the given objectives are complete
func (c *CompletionMonitor) done(ids []protocols.ObjectiveId) bool {

	for _, id := range ids {
		isComplete, ok := c.completed.Load(string(id))

		if !ok || !isComplete.(bool) {
			return false
		}
	}
	return true

}

// watch runs in a gofunc and listens to the CompletedObjectives chan
func (c *CompletionMonitor) watch() {
	for {
		select {
		case id := <-c.client.CompletedObjectives():
			c.completed.Store(string(id), true)
		case <-c.quit:
			return
		// It is important to read from client.ReceivedVouchers otherwise the client can get blocked
		case v := <-c.client.ReceivedVouchers():
			c.log("Received payment of %d wei on channel %s", v.Amount.Int64(), v.ChannelId)
		}
	}
}

// WaitForObjectivesToComplete blocks until all objectives are completed
func (c *CompletionMonitor) WaitForObjectivesToComplete(ids []protocols.ObjectiveId) {
	for {

		if c.done(ids) {

			break
		}
		time.Sleep(SLEEP_TIME)
	}
}

// Close stops the completion monitor from listening to the CompletedObjectives chan
func (c *CompletionMonitor) Close() {
	close(c.quit)
}
