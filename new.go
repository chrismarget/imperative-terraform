package imperative

import (
	"context"
	"log"

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

func NewServer(opts ...ServerOpt) *Server {
	server := Server{ // defaults
		logFunc: log.Printf,
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

	// Collect resource and data source functions from the provider.
	server.loadResourceMap(pmdResp.TypeName)
	server.loadDataSourceMap(pmdResp.TypeName)

	return &server
}
