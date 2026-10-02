package anonymize

import "testing"

func TestApply_Secrets(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"kv equals", "password=hunter2 user=bob", "password=<SECRET> user=bob"},
		{"kv colon", "token: abc123", "token: <SECRET>"},
		{"kv upper key", "API_KEY=xyz", "API_KEY=<SECRET>"},
		{"kv ampersand end", "access_token=abc&x=1", "access_token=<SECRET>&x=1"},
		{"kv prefix key", "client_secret=s3cr3t", "client_secret=<SECRET>"},
		{"kv dash prefix semicolon", "db-password=pw;", "db-password=<SECRET>;"},
		{"kv passwd", "passwd=abc", "passwd=<SECRET>"},
		{"kv pwd", "pwd=abc", "pwd=<SECRET>"},
		{"kv apikey", "apikey=abc", "apikey=<SECRET>"},
		{"kv comma end", "token=abc,next", "token=<SECRET>,next"},
		{"kv quote end", `msg="token=abc"`, `msg="token=<SECRET>"`},
		{"json", `{"password":"hunter2","user":"bob"}`, `{"password":"<SECRET>","user":"bob"}`},
		{"json spaced", `"api_key": "x y"`, `"api_key": "<SECRET>"`},
		{"json prefixed key", `{"refresh_token":"abc"}`, `{"refresh_token":"<SECRET>"}`},
		{"authorization basic", "Authorization: Basic dXNlcjpwYXNz", "Authorization: <SECRET>"},
		{"authorization lower", "authorization: Bearer abc.def", "authorization: <SECRET>"},
		{"authorization quoted", `"Authorization: Basic dXNlcjpwYXNz" ok`, `"Authorization: <SECRET>" ok`},
		{"bearer", "bearer abc.def-123 done", "bearer <SECRET> done"},
		{"bearer upper", "Bearer ABC", "Bearer <SECRET>"},
		{"url credentials", "https://user:pass@example.com/x", "https://<SECRET>@example.com/x"},
		{"aws key", "key AKIAABCDEFGHIJKLMNOP used", "key <SECRET> used"},
		{
			"jwt",
			"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl",
			"<SECRET>",
		},
		{
			"private key block",
			"-----BEGIN RSA PRIVATE KEY-----\nMIIB\n-----END RSA PRIVATE KEY-----",
			"<SECRET>",
		},
		{
			"private key block with surrounding text",
			"before -----BEGIN PRIVATE KEY-----\nMIIB\n-----END PRIVATE KEY----- after",
			"before <SECRET> after",
		},
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

func TestApply_SecretNearMisses(t *testing.T) {
	inputs := []string{
		"tokens_used=5",
		"secretary=alice",
		"tokenizer: cl100k",
		"Bearer",
		"password=<SECRET>",
		`"password":"<SECRET>"`,
		"mailto:bob@example.com",
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
