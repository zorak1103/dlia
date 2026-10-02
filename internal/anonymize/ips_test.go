package anonymize

import "testing"

func TestApply_IPs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"two ips with port", "from 10.0.0.5:8080 to 10.0.0.9", "from <IP-1>:8080 to <IP-2>"},
		{"repeat in message", "10.0.0.5 then 10.0.0.9 then 10.0.0.5", "<IP-1> then <IP-2> then <IP-1>"},
		{"ipv6 zone brackets", "[fe80::1%eth0]:443", "[<IP-1>]:443"},
		{"ipv6", "2001:db8::42", "<IP-1>"},
		{"ipv6 full", "2001:0db8:0000:0000:0000:0000:0000:0042 up", "<IP-1> up"},
		{"public", "203.0.113.7", "<IP-1>"},
		{"broadcast", "255.255.255.255", "<IP-1>"},
		{"ipv6 sentence colon", "peer 2001:db8::42: refused", "peer <IP-1>: refused"},
		{"ipv4 sentence colon", "peer 10.0.0.5: refused", "peer <IP-1>: refused"},
		{"trailing dot", "connect to 10.0.0.5.", "connect to <IP-1>."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSession(Options{IPs: true, Secrets: true})
			if got := s.Apply(tc.input); got != tc.want {
				t.Errorf("Apply(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestApply_IPConsistentAcrossCalls(t *testing.T) {
	s := NewSession(Options{IPs: true, Secrets: true})
	if got, want := s.Apply("from 10.0.0.5"), "from <IP-1>"; got != want {
		t.Fatalf("first call = %q, want %q", got, want)
	}
	if got, want := s.Apply("to 10.0.0.9"), "to <IP-2>"; got != want {
		t.Fatalf("second call = %q, want %q", got, want)
	}
	if got, want := s.Apply("again 10.0.0.5"), "again <IP-1>"; got != want {
		t.Fatalf("third call = %q, want %q", got, want)
	}
}

func TestSession_NewSessionRestartsNumbering(t *testing.T) {
	a := NewSession(Options{IPs: true})
	if got := a.Apply("10.0.0.9"); got != "<IP-1>" {
		t.Fatalf("session A = %q, want <IP-1>", got)
	}
	b := NewSession(Options{IPs: true})
	if got := b.Apply("10.0.0.5"); got != "<IP-1>" {
		t.Fatalf("session B = %q, want <IP-1>", got)
	}
}

func TestApply_IPExceptions(t *testing.T) {
	inputs := []string{"127.0.0.1", "127.8.9.10", "::1", "0.0.0.0:80", "[::]:8080", "listen [::1]:80"}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			s := NewSession(Options{IPs: true, Secrets: true})
			if got := s.Apply(in); got != in {
				t.Errorf("Apply(%q) = %q, want unchanged", in, got)
			}
		})
	}
}

func TestApply_NonIPs(t *testing.T) {
	inputs := []string{
		"999.1.1.1",
		"aa:bb:cc:dd:ee:ff",
		"deadbeefcafe",
		"1.2.3",
		"took 10:15:30",
		"std::cout",
		"a::b::c",
		"1.2.3.4.5",
		"11.2.3.4.5",
		"v1.10.0.0.5",
		"1234.1.1.1",
		"ip:2001:db8::1",
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			s := NewSession(Options{IPs: true, Secrets: true})
			if got := s.Apply(in); got != in {
				t.Errorf("Apply(%q) = %q, want unchanged", in, got)
			}
		})
	}
}

func TestApply_TimestampsUntouched(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			"2026-10-02T09:12:02Z [error] dial tcp 172.18.0.3:5432",
			"2026-10-02T09:12:02Z [error] dial tcp <IP-1>:5432",
		},
		{
			`10.0.0.5 - - [02/Oct/2026:11:00:00 +0000] "GET /"`,
			`<IP-1> - - [02/Oct/2026:11:00:00 +0000] "GET /"`,
		},
		{"took 10:15:30", "took 10:15:30"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			s := NewSession(Options{IPs: true, Secrets: true})
			if got := s.Apply(tc.input); got != tc.want {
				t.Errorf("Apply(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestApply_IPv4MappedIPv6(t *testing.T) {
	s := NewSession(Options{IPs: true, Secrets: true})
	if got := s.Apply("::ffff:10.0.0.1"); got != "<IP-1>" {
		t.Errorf("got %q, want <IP-1>", got)
	}
	if got := s.Apply("::ffff:127.0.0.1"); got != "::ffff:127.0.0.1" {
		t.Errorf("mapped loopback = %q, want unchanged", got)
	}
}

func TestApply_IPv6FalsePositives(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"dead::beef", "<IP-1>"},
		{"std::vector", "std::vector"},
		{"1.2.3.4", "<IP-1>"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			s := NewSession(Options{IPs: true, Secrets: true})
			if got := s.Apply(tc.input); got != tc.want {
				t.Errorf("Apply(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
