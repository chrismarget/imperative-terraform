package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chrismarget/imperative-terraform/internal/shutdown"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

const (
	acceptBackoffMS = 50
	socketPath      = "imperative-terraform-server.sock"
)

type Server struct {
	config            serverConfig
	sockPath          string
	provider          provider.Provider
	sc                *shutdown.Controller
	logFunc           func(string, ...any)
	resources         map[string]bool
	dataSources       map[string]bool
	resourceFuncs     map[string]func() resource.Resource
	dataSourceFuncs   map[string]func() datasource.DataSource
	resourceSchemas   map[string]*resourceSchema.Schema
	dataSourceSchemas map[string]*dataSourceSchema.Schema
	providerVersion   string
	providerConfig    json.RawMessage
	idleTimeout       time.Duration
	graceTimeout      time.Duration
	dataSourceData    any
	resourceData      any
}

func (s *Server) Serve(ctx context.Context) error {
	// Read the provider configuration, daemon discovery file path, and shared
	// secret from stdin. Configure the provider.
	err := s.configure(ctx)
	if err != nil {
		return err
	}

	// Create shutdown controller with a running idle timer.
	s.sc = shutdown.New(
		shutdown.WithDiscoveryFilePath(s.config.DiscoveryFile),
		shutdown.WithLogFunc(s.logFunc),
		shutdown.WithTimeouts(s.idleTimeout, s.graceTimeout),
	)

	// Invent a socket file path if necessary.
	cleanup, err := s.createSockFilePath()
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	// Start the socket listener, Ensure umask is at least 0077 without
	// weakening any existing restrictions.
	syscall.Umask(syscall.Umask(0o7777) | 0o0077)
	listener, err := net.Listen("unix", s.sockPath)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.sockPath, err)
	}

	// Announce that we've started to the bootstrap client via stdout.
	if err := s.announceStartup(); err != nil {
		s.closeListener(listener)
		return err
	}

	// Capture shutdown signals for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// Accept loop - select on shutdown signal or new connection
	for {
		select {
		case <-s.sc.Shutdown():
			// Grace period expired, stop accepting new connections
			s.closeListener(listener)
			s.sc.Wait() // Block until all clients disconnect
			return nil

		case <-ctx.Done():
			// Context canceled, close listener and wait for clients
			s.closeListener(listener)
			s.sc.Wait()
			return ctx.Err()

		case sig := <-sigCh:
			// OS signal received, close listener and wait for clients
			s.closeListener(listener)
			s.logFunc("server: signal %s received: shutting down...", sig)
			s.sc.Wait()
			return nil

		default:
			// Try to accept a connection (non-blocking via short deadline)
			_ = listener.(*net.UnixListener).SetDeadline(time.Now().Add(100 * time.Millisecond))
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				// Check if it's a timeout (expected in our loop)
				if netErr, ok := acceptErr.(net.Error); ok && netErr.Timeout() {
					continue
				}
				// Check if listener was closed
				if errors.Is(acceptErr, net.ErrClosed) || errors.Is(acceptErr, os.ErrClosed) {
					s.sc.Wait()
					return nil
				}
				// Transient accept errors—log and backoff
				s.logFunc("server: accept error: %v", acceptErr)
				time.Sleep(acceptBackoffMS * time.Millisecond)
				continue
			}

			// Track the connection in the shutdown controller and spin out a handler goroutine.
			s.sc.NewClient()
			go s.handleConnection(ctx, conn, s.sc)
		}
	}
}

// closeListener best-effort closes the listener and logs unexpected errors.
func (s *Server) closeListener(listener net.Listener) {
	if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, os.ErrClosed) {
		s.logFunc("server: closing listener: %v", err)
	}
}
