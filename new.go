package imperative

import (
	"context"
	"log"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/provider"
)

type ServerOpt func(*Server)

func WithAPITimeout(d time.Duration) ServerOpt {
	return func(s *Server) {
		s.apiTimeout = d
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

func NewServer(opts ...ServerOpt) *Server {
	server := Server{ // defaults
		apiTimeout: apiTimeoutDefault,
		logFunc:    log.Printf,
	}

	for _, opt := range opts {
		opt(&server)
	}

	if server.provider == nil {
		panic("provider must be specified using NewServer(WithProvider(...))")
	}

	// Gather provider metadata (we need its name)
	var pmdResp provider.MetadataResponse
	server.provider.Metadata(context.Background(), provider.MetadataRequest{}, &pmdResp)

	// Collect resource and data source functions from the provider.
	server.loadResourceMap(pmdResp.TypeName)
	server.loadDataSourceMap(pmdResp.TypeName)

	return &server
}
