package sanitize

import "testing"

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
		{"", ""},
		{"no-slash_here.ok", "no-slash_here.ok"},
	}
	for _, tc := range tests {
		if got := Name(tc.input); got != tc.want {
			t.Errorf("Name(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
