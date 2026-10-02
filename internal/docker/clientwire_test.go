package docker

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDockerDaemon is an httptest server that mimics the Docker Engine API
// endpoints used by the wire-level client in client.go: /_ping,
// /containers/json, and /containers/{id}/logs. It captures the last request
// query per endpoint so tests can pin the wire format the client produces.
type fakeDockerDaemon struct {
	server *httptest.Server

	mu sync.Mutex
	// injectable responses
	pingStatus int
	listStatus int
	logsStatus int
	listBody   string
	logsBody   string
	// captured requests
	lastListQuery url.Values
	lastLogsQuery url.Values
	lastLogsPath  string
	pingCalls     int
}

func newFakeDockerDaemon(t *testing.T, mutate func(*fakeDockerDaemon)) *fakeDockerDaemon {
	t.Helper()
	fake := &fakeDockerDaemon{
		pingStatus: http.StatusOK,
		listStatus: http.StatusOK,
		logsStatus: http.StatusOK,
		listBody: `[{"Id":"abc123def4567890","Names":["/web-1"],"State":"running","Image":"nginx:latest","Labels":{"env":"test"}},
{"Id":"def4567890123456","Names":["/db-1"],"State":"exited","Image":"postgres:16","Labels":{}}]`,
		logsBody: "2025-01-01T10:00:00.000000001Z log line one\n2025-01-01T10:01:00.000000002Z log line two\n",
	}
	if mutate != nil {
		mutate(fake)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", fake.route)
	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)
	return fake
}

// route normalizes the versioned path prefix (/v1.51/...) the Docker client
// prepends, then dispatches on the bare endpoint path.
func (f *fakeDockerDaemon) route(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if idx := strings.Index(path, "/v"); idx == 0 {
		if rest := versionPrefixRe.ReplaceAllString(path, "/"); rest != path {
			path = rest
		}
	}

	switch {
	case strings.HasSuffix(path, "/_ping"):
		f.mu.Lock()
		f.pingCalls++
		status := f.pingStatus
		f.mu.Unlock()
		w.Header().Set("API-Version", "1.45")
		w.Header().Set("Ostype", "linux")
		w.WriteHeader(status)
	case strings.HasSuffix(path, "/containers/json"):
		f.mu.Lock()
		f.lastListQuery = r.URL.Query()
		status, body := f.listStatus, f.listBody
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	case strings.Contains(path, "/containers/") && strings.HasSuffix(path, "/logs"):
		f.mu.Lock()
		f.lastLogsQuery = r.URL.Query()
		f.lastLogsPath = path
		status, body := f.logsStatus, f.logsBody
		f.mu.Unlock()
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeDockerDaemon) listQuery() url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastListQuery
}

func (f *fakeDockerDaemon) logsQuery() url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastLogsQuery
}

func (f *fakeDockerDaemon) pingCallsValue() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pingCalls
}

var versionPrefixRe = regexp.MustCompile(`^/v[0-9][0-9.]*/`)

// TestNewClient_Constructor verifies NewClient constructs a usable Client both
// with the default socket (empty path, no connection attempted) and pointed at
// the fake daemon URL.
func TestNewClient_Constructor(t *testing.T) {
	fake := newFakeDockerDaemon(t, nil)

	defaultClient, err := NewClient("")
	require.NoError(t, err)
	require.NotNil(t, defaultClient)
	require.NoError(t, defaultClient.Close())

	urlClient, err := NewClient(fake.server.URL)
	require.NoError(t, err)
	require.NotNil(t, urlClient)
	require.NoError(t, urlClient.Close())
}

// TestClientWire_Ping_Success verifies Ping returns nil against a 200 from
// the daemon's /_ping endpoint.
func TestClientWire_Ping_Success(t *testing.T) {
	fake := newFakeDockerDaemon(t, nil)

	c, err := NewClient(fake.server.URL)
	require.NoError(t, err)

	require.NoError(t, c.Ping(t.Context()))
	assert.Positive(t, fake.pingCallsValue())
}

// TestClientWire_Ping_HTTPError verifies a non-200 /_ping response is wrapped
// with the socket path in the error text.
func TestClientWire_Ping_HTTPError(t *testing.T) {
	fake := newFakeDockerDaemon(t, func(f *fakeDockerDaemon) {
		f.pingStatus = http.StatusInternalServerError
	})

	c, err := NewClient(fake.server.URL)
	require.NoError(t, err)

	err = c.Ping(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to ping Docker daemon at "+fake.server.URL)
}

// TestClientWire_Ping_ConnectionRefused verifies a dead daemon address yields
// the same wrapper text around the transport error.
func TestClientWire_Ping_ConnectionRefused(t *testing.T) {
	// Port 1 (tcpmux) refuses connections in practice; no listener is started.
	c, err := NewClient("http://127.0.0.1:1")
	require.NoError(t, err)

	err = c.Ping(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to ping Docker daemon at http://127.0.0.1:1")
}

// TestClientWire_ListContainers_AllParam pins the `all` query parameter for
// IncludeAll true/false on the /containers/json request.
func TestClientWire_ListContainers_AllParam(t *testing.T) {
	tests := []struct {
		name       string
		includeAll bool
		wantAll    string
	}{
		{name: "include all sends all=1", includeAll: true, wantAll: "1"},
		{name: "running only omits all", includeAll: false, wantAll: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeDockerDaemon(t, nil)
			c, err := NewClient(fake.server.URL)
			require.NoError(t, err)

			_, err = c.ListContainers(t.Context(), FilterOptions{IncludeAll: tt.includeAll})
			require.NoError(t, err)

			assert.Equal(t, tt.wantAll, fake.listQuery().Get("all"))
		})
	}
}

// TestClientWire_ListContainers_FieldsAndFilters covers field mapping and the
// name-pattern filter over the fake daemon's container list.
func TestClientWire_ListContainers_FieldsAndFilters(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		wantNames []string
		wantErr   string
	}{
		{
			name:      "pattern keeps matches drops non-matches",
			pattern:   `^web-`,
			wantNames: []string{"web-1"},
		},
		{
			name:      "pattern matching everything",
			pattern:   `-1$`,
			wantNames: []string{"web-1", "db-1"},
		},
		{
			name:      "pattern with no matches",
			pattern:   `^nonexistent-`,
			wantNames: []string{},
		},
		{
			name:    "invalid regex",
			pattern: `[`,
			wantErr: "invalid name pattern '['",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeDockerDaemon(t, nil)
			c, err := NewClient(fake.server.URL)
			require.NoError(t, err)

			containers, err := c.ListContainers(t.Context(), FilterOptions{
				IncludeAll:  true,
				NamePattern: tt.pattern,
			})

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)

			names := make([]string, len(containers))
			for i, c := range containers {
				names[i] = c.Name
			}
			assert.Equal(t, tt.wantNames, names)
		})
	}
}

// TestClientWire_ListContainers_FieldMapping pins the mapping of daemon JSON
// onto our Container struct for a matching container.
func TestClientWire_ListContainers_FieldMapping(t *testing.T) {
	fake := newFakeDockerDaemon(t, nil)
	c, err := NewClient(fake.server.URL)
	require.NoError(t, err)

	containers, err := c.ListContainers(t.Context(), FilterOptions{NamePattern: `^web-1$`})
	require.NoError(t, err)
	require.Len(t, containers, 1)

	assert.Equal(t, "abc123def4567890", containers[0].ID)
	assert.Equal(t, "web-1", containers[0].Name) // leading slash stripped
	assert.Equal(t, "running", containers[0].State)
	assert.Equal(t, "nginx:latest", containers[0].Image)
	assert.Equal(t, map[string]string{"env": "test"}, containers[0].Labels)
}

// TestClientWire_ListContainers_NameBoundaries pins the len(Names)==1 boundary:
// zero-length Names keeps the empty name, and a single empty name string skips
// the leading-slash strip instead of mis-slicing.
func TestClientWire_ListContainers_NameBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		listBody string
		wantName string
	}{
		{
			name:     "zero-length Names keeps empty name",
			listBody: `[{"Id":"id000000000000000","Names":[],"State":"running","Image":"busybox"}]`,
			wantName: "",
		},
		{
			name:     "single empty name string skips slash strip",
			listBody: `[{"Id":"id000000000000000","Names":[""],"State":"running","Image":"busybox"}]`,
			wantName: "",
		},
		{
			name:     "exactly one name is used",
			listBody: `[{"Id":"id000000000000000","Names":["/solo"],"State":"running","Image":"busybox"}]`,
			wantName: "solo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeDockerDaemon(t, func(f *fakeDockerDaemon) { f.listBody = tt.listBody })
			c, err := NewClient(fake.server.URL)
			require.NoError(t, err)

			containers, err := c.ListContainers(t.Context(), FilterOptions{IncludeAll: true})
			require.NoError(t, err)
			require.Len(t, containers, 1)
			assert.Equal(t, tt.wantName, containers[0].Name)
		})
	}
}

// TestClientWire_ListContainers_HTTPError verifies a daemon failure on
// /containers/json is wrapped with the socket path.
func TestClientWire_ListContainers_HTTPError(t *testing.T) {
	fake := newFakeDockerDaemon(t, func(f *fakeDockerDaemon) {
		f.listStatus = http.StatusInternalServerError
	})
	c, err := NewClient(fake.server.URL)
	require.NoError(t, err)

	_, err = c.ListContainers(t.Context(), FilterOptions{IncludeAll: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list containers from socket "+fake.server.URL)
}

// TestClientWire_ReadLogsSince_Success verifies the daemon log stream flows
// through parseLogStream into parsed entries, and pins the request query the
// client produces.
func TestClientWire_ReadLogsSince_Success(t *testing.T) {
	fake := newFakeDockerDaemon(t, nil)
	c, err := NewClient(fake.server.URL)
	require.NoError(t, err)

	since := time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)
	entries, err := c.ReadLogsSince(t.Context(), "abc123def4567890", since)
	require.NoError(t, err)

	require.Len(t, entries, 2)
	assert.Equal(t, "2025-01-01T10:00:00.000000001Z", entries[0].Timestamp)
	assert.Equal(t, "log line one", entries[0].Message)
	assert.Equal(t, "stdout", entries[0].Stream)
	assert.Equal(t, "log line two", entries[1].Message)

	assert.Contains(t, fake.lastLogsPath, "/containers/abc123def4567890/logs")
	q := fake.logsQuery()
	assert.Equal(t, "1", q.Get("timestamps"))

	gotSince, parseErr := strconv.ParseFloat(q.Get("since"), 64)
	require.NoError(t, parseErr, "since param must be an epoch float")
	// docker client v28 converts the RFC3339Nano Since option to a Unix epoch
	// float on the wire (timetypes.GetTimestamp); pin the exact value.
	assert.InDelta(t, float64(since.Unix()), gotSince, 0.000000001)
}

// TestClientWire_ReadLogsSince_HTTPError verifies a daemon failure on
// /containers/{id}/logs is wrapped with the container ID.
func TestClientWire_ReadLogsSince_HTTPError(t *testing.T) {
	fake := newFakeDockerDaemon(t, func(f *fakeDockerDaemon) {
		f.logsStatus = http.StatusInternalServerError
	})
	c, err := NewClient(fake.server.URL)
	require.NoError(t, err)

	_, err = c.ReadLogsSince(t.Context(), "abc123def4567890", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read logs for container abc123def4567890")
}

// TestClientWire_ReadLogsLookback_SinceArithmetic pins the lookback
// arithmetic: the wire `since` param must be now-minus-lookback (guards the
// INVERT_NEGATIVES and ARITHMETIC_BASE mutant classes), within a one-second
// tolerance for clock movement between the call and the assertion.
// Note: docker client v28 converts the RFC3339Nano Since option to a Unix
// epoch float on the wire (timetypes.GetTimestamp); we pin the epoch value.
func TestClientWire_ReadLogsLookback_SinceArithmetic(t *testing.T) {
	fake := newFakeDockerDaemon(t, nil)
	c, err := NewClient(fake.server.URL)
	require.NoError(t, err)

	lookback := 2 * time.Hour
	before := time.Now()
	entries, err := c.ReadLogsLookback(t.Context(), "abc123def4567890", lookback)
	after := time.Now()
	require.NoError(t, err)
	require.Len(t, entries, 2)

	sinceFloat, err := strconv.ParseFloat(fake.logsQuery().Get("since"), 64)
	require.NoError(t, err, "since param must be an epoch float")
	gotSince := time.Unix(0, int64(sinceFloat*1e9))

	for _, ref := range []time.Time{before, after} {
		want := ref.Add(-lookback)
		assert.LessOrEqual(t, want.Add(-time.Second), gotSince)
		assert.LessOrEqual(t, gotSince, want.Add(time.Second))
	}
}

// TestNewClient_ConstructorError is documented as unreachable-by-design: the
// docker client's NewClientWithOpts does not fail for any WithHost input we can
// construct (malformed hosts are only rejected at request time), matching the
// phase-1 precedent for unreachable lines.
func TestNewClient_ConstructorError(t *testing.T) {
	t.Skip("client.NewClientWithOpts failure path unreachable via public API; documented, not chased (spec: unreachable-by-design)")
}

const proxyForbiddenBody = "<html><body><h1>403 Forbidden</h1>\nRequest forbidden by administrative rules.\n</body></html>\n"

// TestClientWire_ReadLogs_ProxyDenied_AddsHint verifies a socket-proxy 403
// (HTML body, not Docker JSON) on the logs endpoint carries the proxy hint.
func TestClientWire_ReadLogs_ProxyDenied_AddsHint(t *testing.T) {
	fake := newFakeDockerDaemon(t, func(f *fakeDockerDaemon) {
		f.logsStatus = http.StatusForbidden
		f.logsBody = proxyForbiddenBody
	})
	c, err := NewClient(fake.server.URL)
	require.NoError(t, err)

	_, err = c.ReadLogsSince(t.Context(), "abc123def4567890", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read logs for container")
	assert.Contains(t, err.Error(), "ALLOW_LOGS=1")
}

// TestClientWire_ListContainers_ProxyDenied_AddsHint verifies a socket-proxy
// 403 on /containers/json carries the proxy hint.
func TestClientWire_ListContainers_ProxyDenied_AddsHint(t *testing.T) {
	fake := newFakeDockerDaemon(t, func(f *fakeDockerDaemon) {
		f.listStatus = http.StatusForbidden
		f.listBody = proxyForbiddenBody
	})
	c, err := NewClient(fake.server.URL)
	require.NoError(t, err)

	_, err = c.ListContainers(t.Context(), FilterOptions{IncludeAll: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list containers from socket")
	assert.Contains(t, err.Error(), proxyDeniedHint)
}

// TestClientWire_ReadLogs_ServerError_NoHint verifies non-403 failures do not
// get the proxy hint.
func TestClientWire_ReadLogs_ServerError_NoHint(t *testing.T) {
	fake := newFakeDockerDaemon(t, func(f *fakeDockerDaemon) {
		f.logsStatus = http.StatusInternalServerError
	})
	c, err := NewClient(fake.server.URL)
	require.NoError(t, err)

	_, err = c.ReadLogsSince(t.Context(), "abc123def4567890", time.Now())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "ALLOW_LOGS")
}
