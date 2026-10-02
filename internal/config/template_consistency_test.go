package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zorak1103/dlia/internal/templates"
)

// TestConfigTemplate_ExtraBodyReservedKeysDocumented pins the init template's
// "Not allowed" line to the keys validateExtraBody actually rejects, so the
// template docs cannot drift from the validation list.
func TestConfigTemplate_ExtraBodyReservedKeysDocumented(t *testing.T) {
	var notAllowed string
	for _, line := range strings.Split(string(templates.ConfigYAML), "\n") {
		if strings.Contains(line, "Not allowed:") {
			notAllowed = line
			break
		}
	}
	require.NotEmpty(t, notAllowed, "config template must document the reserved extra_body keys")

	for key := range reservedExtraBodyKeys {
		assert.Contains(t, notAllowed, key, "reserved key %q must be listed in the template's %q line", key, notAllowed)
	}
}
