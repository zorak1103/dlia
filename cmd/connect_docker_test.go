package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zorak1103/dlia/internal/config"
	"github.com/zorak1103/dlia/internal/docker"
)

const socketWarningPrefix = "Warning: DLIA is using the Docker socket directly"

// captureSocketWarning redirects socketWarningOut into a buffer for one test.
func captureSocketWarning(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	original := socketWarningOut
	socketWarningOut = buf
	t.Cleanup(func() { socketWarningOut = original })
	return buf
}

// stubDockerFactory replaces the newDockerClient seam; it records the socket
// path it was called with and returns the given client and error.
func stubDockerFactory(t *testing.T, client docker.Client, err error) *string {
	t.Helper()
	got := new(string)
	original := newDockerClient
	newDockerClient = func(path string) (docker.Client, error) {
		*got = path
		return client, err
	}
	t.Cleanup(func() { newDockerClient = original })
	return got
}

func socketCfg(path string, suppress bool) *config.Config {
	return &config.Config{Docker: config.DockerConfig{SocketPath: path, SuppressSocketWarning: suppress}}
}

func TestSocketWarningOut_DefaultsToStderr(t *testing.T) {
	assert.Equal(t, io.Writer(os.Stderr), socketWarningOut)
}

func TestConnectDocker_WarnsForLocalSockets(t *testing.T) {
	for _, path := range []string{"unix:///var/run/docker.sock", "npipe:////./pipe/docker_engine"} {
		t.Run(path, func(t *testing.T) {
			buf := captureSocketWarning(t)
			stub := &MockDockerClient{}
			gotPath := stubDockerFactory(t, stub, nil)

			client, err := connectDocker(socketCfg(path, false))

			require.NoError(t, err)
			assert.Same(t, stub, client)
			assert.Equal(t, path, *gotPath)
			want := "Warning: DLIA is using the Docker socket directly (" + path + "), which grants root-equivalent access to the host; " +
				`see README "Docker socket access" for the socket-proxy setup (silence with docker.suppress_socket_warning: true)` + "\n"
			assert.Equal(t, want, buf.String())
		})
	}
}

func TestConnectDocker_NoWarningForRemote(t *testing.T) {
	buf := captureSocketWarning(t)
	gotPath := stubDockerFactory(t, &MockDockerClient{}, nil)

	_, err := connectDocker(socketCfg("tcp://socket-proxy:2375", false))

	require.NoError(t, err)
	assert.Equal(t, "tcp://socket-proxy:2375", *gotPath)
	assert.Empty(t, buf.String())
}

func TestConnectDocker_Suppressed(t *testing.T) {
	buf := captureSocketWarning(t)
	stubDockerFactory(t, &MockDockerClient{}, nil)

	_, err := connectDocker(socketCfg("unix:///var/run/docker.sock", true))

	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestConnectDocker_FactoryError_StillWarnsOnce(t *testing.T) {
	buf := captureSocketWarning(t)
	factoryErr := errors.New("no socket")
	stubDockerFactory(t, nil, factoryErr)

	_, err := connectDocker(socketCfg("unix:///var/run/docker.sock", false))

	require.ErrorIs(t, err, factoryErr)
	assert.Equal(t, 1, strings.Count(buf.String(), socketWarningPrefix))
}
