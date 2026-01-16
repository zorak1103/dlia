// Package sanitize provides functions for sanitizing names for safe filesystem use.
package sanitize

import "strings"

// Name converts container names to filesystem-safe names.
// Currently only handles "/" -> "_" substitution since Docker container names
// can only contain [a-zA-Z0-9][a-zA-Z0-9_.-]* plus "/" as separator.
// If future Docker versions allow additional special characters, extend this function.
func Name(name string) string {
	return strings.ReplaceAll(name, "/", "_")
}
