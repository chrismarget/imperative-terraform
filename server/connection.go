package server

import (
	"context"
	"fmt"
	"io"
	"maps"
	"net"
	"slices"

	iio "github.com/chrismarget/imperative-terraform/internal/io"
	"github.com/chrismarget/imperative-terraform/internal/message"
	"github.com/chrismarget/imperative-terraform/internal/shutdown"
)

func (s *Server) handleConnection(ctx context.Context, conn net.Conn, sc *shutdown.Controller) {
	// On return, ensure the socket is closed and the shutdown controller is notified.
	defer func() {
		err := conn.Close()
		if err != nil {
			s.logFunc("server: closing connection: %v", err)
		}
		sc.ClientDone()
	}()

	// Wrap the connection with buffering to allow safe creation of new decoders.
	bconn := iio.NewBufferedConn(conn)

	// Authenticate the client, as required.
	if s.config.Secret != nil && !s.authClient(bconn) {
		s.logFunc("server: client authentication failure")
		return
	}

	// Greet the client.
	if err := s.hello(bconn); err != nil {
		s.logFunc("server: sending hello: %v", err)
		return
	}

	// Enter message exchange loop.
	s.handleMessages(ctx, bconn)
}

// sendError sends an error response to the client.
func (s *Server) sendError(w io.Writer, errMsg string) {
	if err := message.Write(w, &message.Error{Error: errMsg}); err != nil {
		s.logFunc("server: sending error response: %v", err)
	}
}

// hello writes information about the server to the client.
func (s *Server) hello(w io.Writer) error {
	payload := message.Hello{
		Resources:   slices.Collect(maps.Keys(s.resourceFuncs)),
		DataSources: slices.Collect(maps.Keys(s.dataSourceFuncs)),
	}

	if err := message.Write(w, &payload); err != nil {
		return fmt.Errorf("sending hello: %w", err)
	}

	return nil
}
