package anonymize

import "testing"

func TestApply_MixedSecretAndIP(t *testing.T) {
	s := NewSession(Options{IPs: true, Secrets: true})
	in := "postgres://u:p@10.0.0.5:5432/db"
	want := "postgres://<SECRET>@<IP-1>:5432/db"
	if got := s.Apply(in); got != want {
		t.Errorf("Apply(%q) = %q, want %q", in, got, want)
	}
}

func TestApply_Options(t *testing.T) {
	const in = "login from 10.0.0.5 password=hunter2"
	tests := []struct {
		name string
		opts Options
		want string
	}{
		{"both off", Options{}, in},
		{"secrets only", Options{Secrets: true}, "login from 10.0.0.5 password=<SECRET>"},
		{"ips only", Options{IPs: true}, "login from <IP-1> password=hunter2"},
		{"both on", Options{IPs: true, Secrets: true}, "login from <IP-1> password=<SECRET>"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NewSession(tc.opts).Apply(in); got != tc.want {
				t.Errorf("Apply(%q) = %q, want %q", in, got, tc.want)
			}
		})
	}
}
