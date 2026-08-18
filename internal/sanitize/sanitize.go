// Package sanitize provides functions for sanitizing names for safe filesystem use.
package sanitize

import (
	"strings"

	"github.com/spf13/pathologize"
)

// Name converts container names to filesystem-safe names.
// Docker container names can only contain [a-zA-Z0-9][a-zA-Z0-9_.-]* plus "/"
// as a separator, so "/" is flattened to "_" to preserve distinctness in a
// flat directory layout. The result is then run through pathologize.Clean as
// a defensive layer against Windows-reserved device names (e.g. "con",
// "nul") and oversized names, since Docker itself doesn't forbid those.
func Name(name string) string {
	return pathologize.Clean(strings.ReplaceAll(name, "/", "_"))
}
