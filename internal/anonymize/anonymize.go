// Package anonymize masks IP addresses and secrets in log messages before they
// are sent to an LLM. IPs become numbered placeholders (<IP-1>, <IP-2>, ...)
// that stay consistent within a Session; secrets become <SECRET>.
//
// Masking is best-effort, not a security boundary: patterns cannot recognise
// every secret format, and some harmless text is masked as well. Known false
// positives:
//   - version-like strings such as 1.2.3.4 are valid IPv4 addresses
//   - hex-only words that are valid IPv6 addresses, such as dead::beef
//   - values following a secret-looking key in prose ("invalid token: expired")
//
// Known misses:
//   - IPv6 addresses glued to a preceding word or colon (ip:2001:db8::1)
//   - the Authorization rule masks the rest of the header line, not just the
//     credential
//   - multi-line values (only private key blocks are masked across lines)
package anonymize

import "strconv"

// Options selects what to mask.
type Options struct{ IPs, Secrets bool }

// Session masks log messages for one container in one scan. IP numbering is
// consistent within a session; the mapping never leaves memory. A Session is
// not safe for concurrent use.
type Session struct {
	opts Options
	ips  map[string]int
}

// NewSession creates a Session; IP numbering starts at 1.
func NewSession(opts Options) *Session {
	return &Session{opts: opts, ips: make(map[string]int)}
}

// Apply returns msg with the enabled categories masked. Secrets are masked
// first so that credentials embedded in URLs are gone before IPs are numbered.
func (s *Session) Apply(msg string) string {
	if s.opts.Secrets {
		msg = maskSecrets(msg)
	}
	if s.opts.IPs {
		msg = s.maskIPs(msg)
	}
	return msg
}

func (s *Session) placeholder(addr string) string {
	n, ok := s.ips[addr]
	if !ok {
		n = len(s.ips) + 1
		s.ips[addr] = n
	}
	return "<IP-" + strconv.Itoa(n) + ">"
}
