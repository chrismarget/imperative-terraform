package server

import (
	"context"
	"fmt"
	"sync"

	"github.com/chrismarget/imperative-terraform/internal/diags"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerSchema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

func (s *Server) providerSchema() (providerSchema.Schema, error) {
	var schemaResponse provider.SchemaResponse
	s.provider.Schema(context.Background(), provider.SchemaRequest{}, &schemaResponse)
	err := diags.Handle(schemaResponse.Diagnostics, s.logFunc)
	if err != nil {
		return providerSchema.Schema{}, fmt.Errorf("getting provider schema: %w", err)
	}

	return schemaResponse.Schema, nil
}

var schemaMutex = new(sync.RWMutex)

func (s *Server) resourceSchema(name string) (*resourceSchema.Schema, bool) {
	if schema, ok := s.resourceSchemas[name]; ok {
		return schema, ok
	}

	schemaMutex.Lock()
	defer schemaMutex.Unlock()
	if f, ok := s.resourceFuncs[name]; ok {
		var response resource.SchemaResponse
		f().Schema(context.Background(), resource.SchemaRequest{}, &response)
		err := diags.Handle(response.Diagnostics, s.logFunc)
		if err != nil {
			s.logFunc("getting schema for resource %q: %v", name, err)
			return nil, false
		}

		s.resourceSchemas[name] = &response.Schema
	}

	schema, ok := s.resourceSchemas[name]
	return schema, ok
}

func (s *Server) dataSourceSchema(name string) (*dataSourceSchema.Schema, bool) {
	if schema, ok := s.dataSourceSchemas[name]; ok {
		return schema, ok
	}

	schemaMutex.Lock()
	defer schemaMutex.Unlock()
	if f, ok := s.dataSourceFuncs[name]; ok {
		var response datasource.SchemaResponse
		f().Schema(context.Background(), datasource.SchemaRequest{}, &response)
		err := diags.Handle(response.Diagnostics, s.logFunc)
		if err != nil {
			s.logFunc("getting schema for data source %q: %v", name, err)
			return nil, false
		}

		s.dataSourceSchemas[name] = &response.Schema
	}

	schema, ok := s.dataSourceSchemas[name]
	return schema, ok
}
