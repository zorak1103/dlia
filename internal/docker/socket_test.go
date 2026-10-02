package docker

import (
	"errors"
	"fmt"
	"testing"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/stretchr/testify/assert"
)

func TestIsLocalSocket(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"unix:///var/run/docker.sock", true},
		{"UNIX:///var/run/docker.sock", true},
		{"npipe:////./pipe/docker_engine", true},
		{"NPipe:////./pipe/docker_engine", true},
		{"tcp://h:2375", false},
		{"https://h:2376", false},
		{"http://h", false},
		{"ssh://u@h", false},
		{"", false},
		{"/var/run/docker.sock", false},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			assert.Equal(t, tt.want, IsLocalSocket(tt.host))
		})
	}
}

func TestProxyDeniedHint_Text(t *testing.T) {
	assert.Equal(t, "socket proxy denied the request: enable CONTAINERS=1 and ALLOW_LOGS=1, see README", proxyDeniedHint)
}

func TestProxyHint_PermissionDenied_AddsHintKeepsChain(t *testing.T) {
	err := fmt.Errorf("x: %w", cerrdefs.ErrPermissionDenied)

	got := proxyHint(err)

	assert.Contains(t, got.Error(), proxyDeniedHint)
	assert.Contains(t, got.Error(), "x: ")
	assert.ErrorIs(t, got, cerrdefs.ErrPermissionDenied)
}

func TestProxyHint_OtherError_Unchanged(t *testing.T) {
	other := errors.New("boom")

	assert.Same(t, other, proxyHint(other))
}

func TestProxyHint_Nil(t *testing.T) {
	assert.NoError(t, proxyHint(nil))
}
