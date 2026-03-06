package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net"
	"slices"
	"time"

	"github.com/chrismarget/imperative-terraform/internal/diags"
	io2 "github.com/chrismarget/imperative-terraform/internal/io"
	"github.com/chrismarget/imperative-terraform/internal/message"
	"github.com/chrismarget/imperative-terraform/internal/shutdown"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
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
	bconn := io2.NewBufferedConn(conn)

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

// handleMessages reads client requests and dispatches them until the connection closes or context is done.
func (s *Server) handleMessages(ctx context.Context, conn io.ReadWriter) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Read the next request from the client.
		var msg message.Message
		err := message.Read(conn, &msg)
		if err != nil {
			if err == io.EOF {
				return // Client closed the connection
			}
			s.logFunc("connection: reading client message: %v", err)
			return
		}

		// Dispatch based on message type.
		switch msg.Type {
		case message.TypeGoodbye:
			if err = message.Write(conn, (*message.Goodbye)(nil)); err != nil {
				s.logFunc("connection: writing goodbye: %v", err)
			}
			return
		case message.TypeDataSourceRequest:
			s.handleDataSource(ctx, conn, msg.Payload)
		// case message.TypeResourceCreateRequest:
		//	s.handleResourceCreate(conn, req.Payload)
		// case message.TypeResourceReadRequest:
		//	s.handleResourceRead(conn, req.Payload)
		// case message.TypeResourceUpdateRequest:
		//	s.handleResourceUpdate(conn, req.Payload)
		// case message.TypeResourceDeleteRequest:
		//	s.handleResourceDelete(conn, req.Payload)
		default:
			s.logFunc("connection: unknown message type: %q", msg.Type)
			s.sendError(conn, fmt.Sprintf("unknown message type: %q", msg.Type))
		}
	}
}

// sendError sends an error response to the client.
func (s *Server) sendError(w io.Writer, errMsg string) {
	if err := message.Write(w, &message.Error{Error: errMsg}); err != nil {
		s.logFunc("server: sending error response: %v", err)
	}
}

// Placeholder handlers for CRUD operations - to be implemented.
func (s *Server) handleDataSource(ctx context.Context, w io.Writer, payload json.RawMessage) {
	// Unpack the incoming message
	var msg message.DataSourceRequest
	if err := message.UnpackPayload(&msg, payload); err != nil {
		s.sendError(w, fmt.Sprintf("invalid %s", message.TypeDataSourceRequest))
		s.logFunc("handleDataSource: reading %s: %v", message.TypeDataSourceRequest, err)
		return
	}

	// Find the function which returns the required DataSource.
	dsFunc, ok := s.dataSourceFuncs[msg.Name]
	if !ok {
		s.sendError(w, fmt.Sprintf("invalid data source name %q", msg.Name))
		return
	}

	// Instantiate and configure (if required) the DataSource.
	ds := dsFunc()
	if ds, ok := ds.(datasource.DataSourceWithConfigure); ok {
		req := datasource.ConfigureRequest{ProviderData: s.dataSourceData}
		var resp datasource.ConfigureResponse
		ds.Configure(ctx, req, &resp)
		if err := diags.Handle(resp.Diagnostics, s.logFunc); err != nil {
			s.sendError(w, "internal error: configuring data source")
			return
		}

	}

	// Extract the DataSource schema.
	schema, ok := s.dataSourceSchema(msg.Name)
	if !ok {
		s.sendError(w, fmt.Sprintf("invalid data source name %s", msg.Name))
		return
	}

	// Convert the client-specified DataSource config into a tftypes.Type.
	raw, err := tftypes.ValueFromJSON(msg.Config, schema.Type().TerraformType(ctx))
	if err != nil {
		s.sendError(w, "config: internal error")
		s.logFunc("handleDataSource: ValueFromJSON: %v", err)
		return
	}

	req := datasource.ReadRequest{Config: tfsdk.Config{Raw: raw, Schema: schema}}
	resp := datasource.ReadResponse{
		State: tfsdk.State{
			Raw:    tftypes.Value{},
			Schema: schema,
		},
		Diagnostics: nil,
		Deferred:    nil,
	}
	deadline, cancel := context.WithDeadline(ctx, time.Now().Add(s.config.APITimeout))
	defer cancel()
	ds.Read(deadline, req, &resp)
	err = diags.Handle(resp.Diagnostics, s.logFunc)
	if err != nil {
		s.sendError(w, "internal error: data source read")
		s.logFunc("handleDataSource: Read: %v", err)
		return
	}

	//var state interface{}
	//if err := resp.State.Raw.As(&state); err != nil {
	//	s.sendError(w, "internal error: marshaling state")
	//	s.logFunc("handleDataSource: State.Raw.As: %v", err)
	//	return
	//}
}

//// ValueToTerraformJSON takes a framework Value and produces Terraform‑compatible JSON.
//func ValueToTerraformJSON(ctx context.Context, val tftypes.Value) ([]byte, error) {
//	// Convert the framework value to a cty.Value
//	ctyp, err := val.As(cty.DynamicPseudoType)
//	if err != nil {
//		return nil, err
//	}
//
//	// Now convert the cty.Value to a Go representation
//	goVal, err := gocty.CtyValueToGoValue(ctyp, reflect.TypeOf(interface{}(nil)))
//	if err != nil {
//		return nil, err
//	}
//
//	// Finally marshal to JSON
//	return json.Marshal(goVal)
//}

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
