package server

import (
	"fmt"
	"os"
	"time"
)

type serverConfig struct {
	Secret        []byte        `json:"secret"`
	DiscoveryFile string        `json:"discovery_file"`
	APITimeout    time.Duration `json:"api_timeout"`
}

func (c serverConfig) validate() error {
	if _, err := os.Stat(c.DiscoveryFile); err != nil {
		return fmt.Errorf("server_config: stat discovery file %q: %w", c.DiscoveryFile, err)
	}

	return nil
}

func defaultServerConfig() serverConfig {
	return serverConfig{
		APITimeout: defaultAPITimeout,
	}
}
