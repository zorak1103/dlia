package sanitize

import (
	"strings"
	"testing"
)

func TestName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"mycontainer", "mycontainer"},
		{"my/container", "my_container"},
		{"a/b/c", "a_b_c"},
		{"/leading", "_leading"},
		{"trailing/", "trailing_"},
		{"", "file"},
		{"no-slash_here.ok", "no-slash_here.ok"},
		// Windows reserved device names are valid Docker container names but
		// unsafe as filenames; pathologize.Clean defuses them.
		{"con", "CON_"},
		{"NUL", "NUL_"},
	}
	for _, tc := range tests {
		if got := Name(tc.input); got != tc.want {
			t.Errorf("Name(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestNameCapsLength(t *testing.T) {
	long := strings.Repeat("a", 400)
	got := Name(long)
	if len(got) > 255 {
		t.Errorf("Name() returned %d bytes, want <= 255", len(got))
	}
}
