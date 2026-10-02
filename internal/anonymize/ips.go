package anonymize

import (
	"net/netip"
	"regexp"
	"strings"
)

var (
	// ipv6Candidate is deliberately loose; netip.ParseAddr does the validation.
	// The dotted-quad alternative comes first so IPv4-mapped forms are matched whole.
	ipv6Candidate = regexp.MustCompile(
		`(?:[0-9A-Fa-f]{0,4}:){2,7}(?:\d{1,3}(?:\.\d{1,3}){3}|[0-9A-Fa-f]{0,4})(?:%[0-9A-Za-z_-]+)?`)
	ipv4Candidate = regexp.MustCompile(`\d{1,3}(?:\.\d{1,3}){3}`)
)

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isWordOrColon(c byte) bool {
	return c == ':' || c == '_' || isDigit(c) || (c|0x20 >= 'a' && c|0x20 <= 'z')
}

// embeddedV4 reports whether msg[start:end] is part of a longer dotted number
// such as 1.2.3.4.5 or 11.2.3.4.
func embeddedV4(msg string, start, end int) bool {
	if start > 0 && (isDigit(msg[start-1]) || msg[start-1] == '.' && start > 1 && isDigit(msg[start-2])) {
		return true
	}
	return end < len(msg) && (isDigit(msg[end]) || msg[end] == '.' && end+1 < len(msg) && isDigit(msg[end+1]))
}

// embeddedV6 reports whether msg[start:end] is part of a longer token such as
// std::vector or a longer colon-separated hex string.
func embeddedV6(msg string, start, end int) bool {
	return start > 0 && isWordOrColon(msg[start-1]) || end < len(msg) && isWordOrColon(msg[end])
}

// maskIPs masks IPv6 candidates first so that IPv4-mapped addresses become a
// single placeholder, then the remaining IPv4 candidates.
func (s *Session) maskIPs(msg string) string {
	msg = s.replaceIPs(msg, ipv6Candidate, true)
	return s.replaceIPs(msg, ipv4Candidate, false)
}

// replaceIPs replaces every candidate of re that is a valid, non-loopback,
// non-unspecified address and not embedded in a longer token.
func (s *Session) replaceIPs(msg string, re *regexp.Regexp, v6 bool) string {
	var b strings.Builder
	last := 0
	for _, loc := range re.FindAllStringIndex(msg, -1) {
		start, end := loc[0], loc[1]
		embedded := embeddedV4
		if v6 {
			embedded = embeddedV6
			// "addr: text" leaves a sentence colon on the candidate; drop it.
			if strings.HasSuffix(msg[start:end], ":") && !strings.HasSuffix(msg[start:end], "::") {
				end--
				embedded = func(m string, s, e int) bool { return s > 0 && isWordOrColon(m[s-1]) }
			}
		}
		if embedded(msg, start, end) {
			continue
		}
		cand := msg[start:end]
		addr, err := netip.ParseAddr(cand)
		if err != nil {
			continue
		}
		if a := addr.Unmap(); a.IsLoopback() || a.IsUnspecified() {
			continue
		}
		b.WriteString(msg[last:start])
		b.WriteString(s.placeholder(cand))
		last = end
	}
	if last == 0 {
		return msg
	}
	b.WriteString(msg[last:])
	return b.String()
}
