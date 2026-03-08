package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/chrismarget/imperative-terraform/internal/diags"
	ijson "github.com/chrismarget/imperative-terraform/internal/json"
	"github.com/chrismarget/imperative-terraform/internal/message"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

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
		case message.TypeResourceRequest:
			s.handleResource(ctx, conn, msg.Payload)
		default:
			s.logFunc("connection: unknown message type: %q", msg.Type)
			s.sendError(conn, fmt.Sprintf("unknown message type: %q", msg.Type))
		}
	}
}

// handleDataSource invokes the datasource.Datasource Read() method and returns the result to the client.
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
			s.logFunc("handleDataSource: configuring data source: %v", err)
			return
		}
	}

	// Extract the DataSource schema.
	schema, ok := s.dataSourceSchema(msg.Name)
	if !ok {
		s.sendError(w, fmt.Sprintf("invalid data source name %s", msg.Name))
		return
	}

	// Convert the client-specified config into a tftypes.Value.
	config, err := ijson.ValueFrom(msg.Config, schema.Type().TerraformType(ctx))
	if err != nil {
		s.sendError(w, "config: internal error")
		s.logFunc("handleDataSource: ValueFrom(config): %v", err)
		return
	}

	// Call the data source Read method with timeout.
	req := datasource.ReadRequest{Config: tfsdk.Config{Raw: config, Schema: schema}}
	resp := datasource.ReadResponse{State: tfsdk.State{Schema: schema}}
	deadline, cancel := context.WithDeadline(ctx, time.Now().Add(s.config.APITimeout))
	defer cancel()
	ds.Read(deadline, req, &resp)
	err = diags.Handle(resp.Diagnostics, s.logFunc)
	if err != nil {
		s.sendError(w, "internal error: data source read")
		s.logFunc("handleDataSource: Read: %v", err)
		return
	}

	// Convert the state value to JSON
	stateJSON, err := ijson.ValueTo(resp.State.Raw)
	if err != nil {
		s.sendError(w, "internal error: marshaling state")
		s.logFunc("handleDataSource: converting state to json: %v", err)
		return
	}

	// Send the response back to the client
	respMsg := message.DataSourceResponse{
		Name:  msg.Name,
		State: json.RawMessage(stateJSON),
	}
	if err := message.Write(w, &respMsg); err != nil {
		s.logFunc("connection: writing data source response: %v", err)
		return
	}
}

// handleResource dispatches resource requests based on the Method field.
func (s *Server) handleResource(ctx context.Context, w io.Writer, payload json.RawMessage) {
	// Unpack the incoming message
	var msg message.ResourceRequest
	if err := message.UnpackPayload(&msg, payload); err != nil {
		s.sendError(w, fmt.Sprintf("invalid %s", message.TypeResourceRequest))
		s.logFunc("handleResource: reading %s: %v", message.TypeResourceRequest, err)
		return
	}

	// Dispatch based on method
	switch msg.Method {
	case "create":
		s.handleResourceCreate(ctx, w, msg)
	case "read":
		s.handleResourceRead(ctx, w, msg)
	case "update":
		s.handleResourceUpdate(ctx, w, msg)
	case "delete":
		s.handleResourceDelete(ctx, w, msg)
	default:
		s.sendError(w, fmt.Sprintf("invalid resource method: %q", msg.Method))
		return
	}
}

// handleResourceCreate handles resource creation.
func (s *Server) handleResourceCreate(ctx context.Context, w io.Writer, msg message.ResourceRequest) {
	// Find the function which returns the required Resource.
	rsrcFunc, ok := s.resourceFuncs[msg.Name]
	if !ok {
		s.sendError(w, fmt.Sprintf("invalid resource name %q", msg.Name))
		return
	}

	// Instantiate and configure (if required) the Resource.
	rsrc := rsrcFunc()
	if rsrc, ok := rsrc.(resource.ResourceWithConfigure); ok {
		req := resource.ConfigureRequest{ProviderData: s.resourceData}
		var resp resource.ConfigureResponse
		rsrc.Configure(ctx, req, &resp)
		if err := diags.Handle(resp.Diagnostics, s.logFunc); err != nil {
			s.sendError(w, "internal error: configuring resource")
			s.logFunc("handleResourceCreate: configuring resource: %v", err)
			return
		}
	}

	// Extract the Resource schema.
	schema, ok := s.resourceSchema(msg.Name)
	if !ok {
		s.sendError(w, fmt.Sprintf("invalid resource name %s", msg.Name))
		return
	}

	// Convert the client-specified config into a tftypes.Value.
	config, err := ijson.ValueFrom(msg.Config, schema.Type().TerraformType(ctx))
	if err != nil {
		s.sendError(w, "config: internal error")
		s.logFunc("handleResourceCreate: ValueFrom(config): %v", err)
		return
	}

	// Convert the client-specified plan into a tftypes.Value.
	plan, err := ijson.ValueFrom(msg.Plan, schema.Type().TerraformType(ctx))
	if err != nil {
		s.sendError(w, "plan: internal error")
		s.logFunc("handleResourceCreate: ValueFrom(plan): %v", err)
		return
	}

	// Call the resource Create method with timeout.
	req := resource.CreateRequest{
		Config: tfsdk.Config{Raw: config, Schema: *schema},
		Plan:   tfsdk.Plan{Raw: plan, Schema: *schema},
	}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: schema}}
	deadline, cancel := context.WithDeadline(ctx, time.Now().Add(s.config.APITimeout))
	defer cancel()
	rsrc.Create(deadline, req, &resp)
	err = diags.Handle(resp.Diagnostics, s.logFunc)
	if err != nil {
		s.sendError(w, "internal error: resource create")
		s.logFunc("handleResourceCreate: Create: %v", err)
		return
	}

	// Convert the state value to JSON
	stateJSON, err := ijson.ValueTo(resp.State.Raw)
	if err != nil {
		s.sendError(w, "internal error: marshaling state")
		s.logFunc("handleResourceCreate: converting state to json: %v", err)
		return
	}

	// Send the response back to the client
	respMsg := message.ResourceResponse{
		Name:  msg.Name,
		State: json.RawMessage(stateJSON),
	}
	if err := message.Write(w, &respMsg); err != nil {
		s.logFunc("connection: writing resource create response: %v", err)
		return
	}
}

// handleResourceRead handles resource read operations.
func (s *Server) handleResourceRead(ctx context.Context, w io.Writer, msg message.ResourceRequest) {
	// Find the function which returns the required Resource.
	rsrcFunc, ok := s.resourceFuncs[msg.Name]
	if !ok {
		s.sendError(w, fmt.Sprintf("invalid resource name %q", msg.Name))
		return
	}

	// Instantiate and configure (if required) the Resource.
	rsrc := rsrcFunc()
	if rsrc, ok := rsrc.(resource.ResourceWithConfigure); ok {
		req := resource.ConfigureRequest{ProviderData: s.resourceData}
		var resp resource.ConfigureResponse
		rsrc.Configure(ctx, req, &resp)
		if err := diags.Handle(resp.Diagnostics, s.logFunc); err != nil {
			s.sendError(w, "internal error: configuring resource")
			s.logFunc("handleResourceRead: configuring resource: %v", err)
			return
		}
	}

	// Extract the Resource schema.
	schema, ok := s.resourceSchema(msg.Name)
	if !ok {
		s.sendError(w, fmt.Sprintf("invalid resource name %s", msg.Name))
		return
	}

	// Convert the client-specified state into a tftypes.Value.
	state, err := ijson.ValueFrom(msg.State, schema.Type().TerraformType(ctx))
	if err != nil {
		s.sendError(w, "state: internal error")
		s.logFunc("handleResourceRead: ValueFrom(state): %v", err)
		return
	}

	// Call the resource Read method with timeout.
	req := resource.ReadRequest{State: tfsdk.State{Raw: state, Schema: *schema}}
	resp := resource.ReadResponse{State: tfsdk.State{Schema: schema}}
	deadline, cancel := context.WithDeadline(ctx, time.Now().Add(s.config.APITimeout))
	defer cancel()
	rsrc.Read(deadline, req, &resp)
	err = diags.Handle(resp.Diagnostics, s.logFunc)
	if err != nil {
		s.sendError(w, "internal error: resource read")
		s.logFunc("handleResourceRead: Read: %v", err)
		return
	}

	// Convert the state value to JSON
	stateJSON, err := ijson.ValueTo(resp.State.Raw)
	if err != nil {
		s.sendError(w, "internal error: marshaling state")
		s.logFunc("handleResourceRead: converting state to json: %v", err)
		return
	}

	// Send the response back to the client
	respMsg := message.ResourceResponse{
		Name:  msg.Name,
		State: json.RawMessage(stateJSON),
	}
	if err := message.Write(w, &respMsg); err != nil {
		s.logFunc("connection: writing resource read response: %v", err)
		return
	}
}

// handleResourceUpdate handles resource update operations.
func (s *Server) handleResourceUpdate(ctx context.Context, w io.Writer, msg message.ResourceRequest) {
	// Find the function which returns the required Resource.
	rsrcFunc, ok := s.resourceFuncs[msg.Name]
	if !ok {
		s.sendError(w, fmt.Sprintf("invalid resource name %q", msg.Name))
		return
	}

	// Instantiate and configure (if required) the Resource.
	rsrc := rsrcFunc()
	if rsrc, ok := rsrc.(resource.ResourceWithConfigure); ok {
		req := resource.ConfigureRequest{ProviderData: s.resourceData}
		var resp resource.ConfigureResponse
		rsrc.Configure(ctx, req, &resp)
		if err := diags.Handle(resp.Diagnostics, s.logFunc); err != nil {
			s.sendError(w, "internal error: configuring resource")
			s.logFunc("handleResourceUpdate: configuring resource: %v", err)
			return
		}
	}

	// Extract the Resource schema.
	schema, ok := s.resourceSchema(msg.Name)
	if !ok {
		s.sendError(w, fmt.Sprintf("invalid resource name %s", msg.Name))
		return
	}

	// Convert the client-specified config into a tftypes.Value.
	config, err := ijson.ValueFrom(msg.Config, schema.Type().TerraformType(ctx))
	if err != nil {
		s.sendError(w, "config: internal error")
		s.logFunc("handleResourceUpdate: ValueFrom(config): %v", err)
		return
	}

	// Convert the client-specified plan into a tftypes.Value.
	plan, err := ijson.ValueFrom(msg.Plan, schema.Type().TerraformType(ctx))
	if err != nil {
		s.sendError(w, "plan: internal error")
		s.logFunc("handleResourceUpdate: ValueFrom(plan): %v", err)
		return
	}

	// Convert the client-specified state into a tftypes.Value.
	state, err := ijson.ValueFrom(msg.State, schema.Type().TerraformType(ctx))
	if err != nil {
		s.sendError(w, "state: internal error")
		s.logFunc("handleResourceUpdate: ValueFrom(state): %v", err)
		return
	}

	// Call the resource Update method with timeout.
	req := resource.UpdateRequest{
		Config: tfsdk.Config{Raw: config, Schema: *schema},
		Plan:   tfsdk.Plan{Raw: plan, Schema: *schema},
		State:  tfsdk.State{Raw: state, Schema: *schema},
	}
	resp := resource.UpdateResponse{State: tfsdk.State{Schema: schema}}
	deadline, cancel := context.WithDeadline(ctx, time.Now().Add(s.config.APITimeout))
	defer cancel()
	rsrc.Update(deadline, req, &resp)
	err = diags.Handle(resp.Diagnostics, s.logFunc)
	if err != nil {
		s.sendError(w, "internal error: resource update")
		s.logFunc("handleResourceUpdate: Update: %v", err)
		return
	}

	// Convert the state value to JSON
	stateJSON, err := ijson.ValueTo(resp.State.Raw)
	if err != nil {
		s.sendError(w, "internal error: marshaling state")
		s.logFunc("handleResourceUpdate: converting state to json: %v", err)
		return
	}

	// Send the response back to the client
	respMsg := message.ResourceResponse{
		Name:  msg.Name,
		State: json.RawMessage(stateJSON),
	}
	if err := message.Write(w, &respMsg); err != nil {
		s.logFunc("connection: writing resource update response: %v", err)
		return
	}
}

// handleResourceDelete handles resource deletion.
func (s *Server) handleResourceDelete(ctx context.Context, w io.Writer, msg message.ResourceRequest) {
	// Find the function which returns the required Resource.
	rsrcFunc, ok := s.resourceFuncs[msg.Name]
	if !ok {
		s.sendError(w, fmt.Sprintf("invalid resource name %q", msg.Name))
		return
	}

	// Instantiate and configure (if required) the Resource.
	rsrc := rsrcFunc()
	if rsrc, ok := rsrc.(resource.ResourceWithConfigure); ok {
		req := resource.ConfigureRequest{ProviderData: s.resourceData}
		var resp resource.ConfigureResponse
		rsrc.Configure(ctx, req, &resp)
		if err := diags.Handle(resp.Diagnostics, s.logFunc); err != nil {
			s.sendError(w, "internal error: configuring resource")
			s.logFunc("handleResourceDelete: configuring resource: %v", err)
			return
		}
	}

	// Extract the Resource schema.
	schema, ok := s.resourceSchema(msg.Name)
	if !ok {
		s.sendError(w, fmt.Sprintf("invalid resource name %s", msg.Name))
		return
	}

	// Convert the client-specified state into a tftypes.Value.
	state, err := ijson.ValueFrom(msg.State, schema.Type().TerraformType(ctx))
	if err != nil {
		s.sendError(w, "state: internal error")
		s.logFunc("handleResourceDelete: ValueFrom(state): %v", err)
		return
	}

	// Call the resource Delete method with timeout.
	req := resource.DeleteRequest{
		State: tfsdk.State{Raw: state, Schema: *schema},
	}
	resp := resource.DeleteResponse{State: tfsdk.State{Schema: schema}}
	deadline, cancel := context.WithDeadline(ctx, time.Now().Add(s.config.APITimeout))
	defer cancel()
	rsrc.Delete(deadline, req, &resp)
	err = diags.Handle(resp.Diagnostics, s.logFunc)
	if err != nil {
		s.sendError(w, "internal error: resource delete")
		s.logFunc("handleResourceDelete: Delete: %v", err)
		return
	}

	// For delete, we send back an empty state (resource no longer exists)
	respMsg := message.ResourceResponse{
		Name:  msg.Name,
		State: json.RawMessage("null"),
	}
	if err := message.Write(w, &respMsg); err != nil {
		s.logFunc("connection: writing resource delete response: %v", err)
		return
	}
}
