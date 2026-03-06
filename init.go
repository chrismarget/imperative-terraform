package imperative

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chrismarget/imperative-terraform/diags"
	"github.com/chrismarget/imperative-terraform/message"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func (s *Server) configureProvider(ctx context.Context) error {
	// Read and unpack the configuration from the client via stdin.
	var config message.Config
	if err := message.Read(os.Stdin, &config); err != nil {
		return fmt.Errorf("init: unpacking config: %w", err)
	}
	if config.DiscoveryFilePath == "" {
		return fmt.Errorf("init: discovery_file_path is required in %s message", message.TypeConfig)
	}
	if _, err := os.Stat(config.DiscoveryFilePath); err != nil {
		return fmt.Errorf("init: stat discovery file path %q: %w", config.DiscoveryFilePath, err)
	}

	// Save client-specified server parameters.
	s.secret = config.Secret
	s.discoveryFilePath = config.DiscoveryFilePath

	// Get the provider schema.
	pSchema, err := s.providerSchema()
	if err != nil {
		return err
	}

	// Convert the client-specified provider configuration into a tftypes.Type.
	raw, err := tftypes.ValueFromJSON(config.ProviderConfig, pSchema.Type().TerraformType(ctx))
	if err != nil {
		return fmt.Errorf("init: parsing provider_config %q to terraform value: %w", config.ProviderConfig, err)
	}

	// Configure the provider.
	configureResponse := new(provider.ConfigureResponse)
	configureRequest := provider.ConfigureRequest{Config: tfsdk.Config{Raw: raw, Schema: pSchema}}
	s.provider.Configure(ctx, configureRequest, configureResponse)
	err = diags.Handle(configureResponse.Diagnostics, s.logFunc)
	if err != nil {
		return fmt.Errorf("init: configuring provider: %w", err)
	}

	return nil
}

// announceStartup writes a listening message to our bootstrap client on stdout.
func (s *Server) announceStartup() error {
	payload := message.Listening{
		AuthNRequired: s.secret != nil,
		ListeningOn:   s.sockPath,
	}

	if err := message.Write(os.Stdout, &payload); err != nil {
		return fmt.Errorf("init: announcing startup: %w", err)
	}
	return nil
}

// createSockFilePath creates a temporary directory and sets s.sockPath to a
// socket file within that directory and returns a cleanup function.
func (s *Server) createSockFilePath() (func(), error) {
	dir, err := os.MkdirTemp("", "")
	if err != nil {
		return nil, fmt.Errorf("init: creating temp dir: %w", err)
	}

	s.sockPath = filepath.Join(dir, socketPath)

	return func() {
		err := os.RemoveAll(dir)
		if err != nil {
			s.logFunc("cleanup: removing temp dir %q: %v", dir, err)
		}
	}, nil
}

func (s *Server) loadDataSourceMap(providerTypeName string) {
	// Collect data source functions from the provider
	req := datasource.MetadataRequest{ProviderTypeName: providerTypeName}
	var resp datasource.MetadataResponse
	s.dataSourceSchemas = make(map[string]*dataSourceSchema.Schema, len(allowedDataSources))
	s.dataSourceFuncs = make(map[string]func() datasource.DataSource, len(allowedDataSources))
	for _, f := range s.provider.DataSources(context.Background()) {
		f().Metadata(context.Background(), req, &resp)
		if allowedDataSources[resp.TypeName] {
			s.dataSourceFuncs[resp.TypeName] = f
		}
	}
}

func (s *Server) loadResourceMap(providerTypeName string) {
	// Collect resource functions from the provider
	req := resource.MetadataRequest{ProviderTypeName: providerTypeName}
	var resp resource.MetadataResponse
	s.resourceSchemas = make(map[string]*resourceSchema.Schema, len(allowedResources))
	s.resourceFuncs = make(map[string]func() resource.Resource, len(allowedResources))
	for _, f := range s.provider.Resources(context.Background()) {
		f().Metadata(context.Background(), req, &resp)
		if allowedResources[resp.TypeName] {
			s.resourceFuncs[resp.TypeName] = f
		}
	}
}
