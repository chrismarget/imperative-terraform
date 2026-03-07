package server

import (
	"context"
	"log"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/provider"
)

type ServerOpt func(*Server)

func WithDataSources(ds []string) ServerOpt {
	return func(s *Server) {
		s.dataSources = make(map[string]bool, len(ds))
		for _, d := range ds {
			s.dataSources[d] = true
		}
	}
}

func WithResources(rs []string) ServerOpt {
	return func(s *Server) {
		s.resources = make(map[string]bool, len(rs))
		for _, r := range rs {
			s.resources[r] = true
		}
	}
}

func WithLogFunc(f func(string, ...any)) ServerOpt {
	return func(s *Server) {
		s.logFunc = f
	}
}

func WithProvider(p provider.Provider) ServerOpt {
	return func(s *Server) {
		s.provider = p
	}
}

func WithSockFile(path string) ServerOpt {
	return func(s *Server) {
		s.sockPath = path
	}
}

func WithTimeouts(idle, grace time.Duration) ServerOpt {
	return func(s *Server) {
		s.idleTimeout = idle
		s.graceTimeout = grace
	}
}

func New(opts ...ServerOpt) *Server {
	server := Server{ // defaults
		logFunc:      log.Printf,
		idleTimeout:  defaultIdleTimeout,
		graceTimeout: defaultGraceTimeout,
		config:       defaultServerConfig(),
	}

	for _, opt := range opts {
		opt(&server)
	}

	if server.provider == nil {
		panic("provider must be specified using NewServer(WithProvider(...))")
	}

	// Gather provider metadata
	var pmdResp provider.MetadataResponse
	server.providerVersion = pmdResp.Version
	server.provider.Metadata(context.Background(), provider.MetadataRequest{}, &pmdResp)

	// Collect data source and resource functions from the provider.
	server.loadDataSourceMap(pmdResp.TypeName)
	server.loadResourceMap(pmdResp.TypeName)

	return &server
}
