package shutdown

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	idleTimeout  = 30 * time.Second
	graceTimeout = 5 * time.Second
)

type Controller struct {
	wg                *sync.WaitGroup
	mu                *sync.Mutex
	clientCount       atomic.Int64
	shutdownChan      chan struct{}
	discoveryFilePath string
	idleTimer         *time.Timer
	graceTimer        *time.Timer
	logFunc           func(string, ...any)
}

func (c *Controller) SetDiscoveryFilePath(path string) {
	if c.discoveryFilePath != "" {
		panic(fmt.Sprintf("trying to change discovery file path from: %q to %q", c.discoveryFilePath, path))
	}
	c.discoveryFilePath = path
}

// NewClient must be called whenever a client connection is established. It increments
// the client count and stops the idle timer if this is the first active client.
func (c *Controller) NewClient() {
	c.wg.Add(1)
	if c.clientCount.Add(1) == 1 {
		// Server has transitioned from idle to active. Stop the idle timer if it was running.
		c.stopIdleTimer()
	}
}

// ClientDone must be called whenever a client connection is closed. It decrements
// the client count and starts the idle timer if this was the last active client.
func (c *Controller) ClientDone() {
	c.wg.Done()
	if c.clientCount.Add(-1) == 0 {
		// Last client has disconnected. Start the idle timer.
		c.startIdleTimer()
	}
}

// Wait blocks until all clients have disconnected.
func (c *Controller) Wait() {
	c.wg.Wait()
}

// Shutdown returns a channel that will be closed when the shutdown idle time + grace period
// have both expired and the server should stop accepting new clients.
func (c *Controller) Shutdown() <-chan struct{} {
	return c.shutdownChan
}

// startIdleTimer begins the idle-timeout countdown. Called when clientCount count reaches zero.
func (c *Controller) startIdleTimer() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.idleTimer == nil {
		c.idleTimer = time.AfterFunc(idleTimeout, c.handleIdleTimeout)
	}
}

// stopIdleTimer stops and resets the idle timer when a client connects to the server.
func (c *Controller) stopIdleTimer() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.idleTimer != nil {
		c.idleTimer.Stop()
		c.idleTimer = nil
	}
}

// handleIdleTimeout is the idle timer callback function. It runs when the idle timer expires,
// indicating that no clients have connected for the duration of the idle timeout.
func (c *Controller) handleIdleTimeout() {
	c.logFunc("shutdown: %s idle timeout expired; removing server discovery file %q now; shutting down socket listener in %s", idleTimeout, c.discoveryFilePath, graceTimeout)
	c.mu.Lock()
	c.idleTimer = nil
	c.mu.Unlock()

	if c.discoveryFilePath != "" {
		err := os.Remove(c.discoveryFilePath)
		if err != nil && !errors.Is(err, os.ErrNotExist) && c.logFunc != nil {
			c.logFunc("daemon discovery file %q: %v", c.discoveryFilePath, err)
		}
	}

	c.beginGracePeriod()
}

// beginGracePeriod stops the idle timer and starts the grace timer.
// Called after lockfile is deleted to allow stragglers to finish.
func (c *Controller) beginGracePeriod() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.idleTimer != nil {
		c.idleTimer.Stop()
		c.idleTimer = nil
	}
	if c.graceTimer == nil {
		c.graceTimer = time.AfterFunc(graceTimeout, c.handleGraceTimeout)
	}
}

func (c *Controller) handleGraceTimeout() {
	c.logFunc("shutdown: %s grace period expired, shutting down server after %d clients disconnect", graceTimeout, c.clientCount.Load())
	c.mu.Lock()
	c.graceTimer = nil
	c.mu.Unlock()
	close(c.shutdownChan) // signal that shutdown is now in effect, so server should stop accepting new clients
}

type ControllerOpt func(*Controller)

func WithDiscoveryFilePath(path string) ControllerOpt {
	return func(c *Controller) {
		c.discoveryFilePath = path
	}
}

func WithLogFunc(f func(string, ...any)) ControllerOpt {
	return func(c *Controller) {
		c.logFunc = f
	}
}

func New(opts ...func(*Controller)) *Controller {
	c := Controller{
		mu:           new(sync.Mutex),
		shutdownChan: make(chan struct{}),
		wg:           new(sync.WaitGroup),
	}

	for _, opt := range opts {
		opt(&c)
	}

	c.startIdleTimer()

	return &c
}
