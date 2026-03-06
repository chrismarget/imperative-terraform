package server

import "time"

const (
	defaultAPITimeout   = 60 * time.Second
	defaultIdleTimeout  = 30 * time.Second
	defaultGraceTimeout = 5 * time.Second
)
