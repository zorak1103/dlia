// Package templates contains embedded template files.
package templates

import (
	_ "embed"
)

//go:embed config.template

// ConfigYAML contains the embedded configuration template.
var ConfigYAML []byte

//go:embed env.template

// EnvFile contains the embedded environment file template.
var EnvFile []byte
