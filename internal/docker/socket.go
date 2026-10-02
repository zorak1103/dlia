package docker

import (
	"fmt"
	"strings"

	cerrdefs "github.com/containerd/errdefs"
)

const proxyDeniedHint = "socket proxy denied the request: enable CONTAINERS=1 and ALLOW_LOGS=1, see README"

// IsLocalSocket reports whether host points at a local Docker socket
// (unix:// or npipe://), which grants root-equivalent access to the host.
func IsLocalSocket(host string) bool {
	h := strings.ToLower(host)
	return strings.HasPrefix(h, "unix://") || strings.HasPrefix(h, "npipe://")
}

// proxyHint appends a socket-proxy hint to permission-denied (HTTP 403) errors.
func proxyHint(err error) error {
	if cerrdefs.IsPermissionDenied(err) {
		return fmt.Errorf("%w (%s)", err, proxyDeniedHint)
	}
	return err
}
