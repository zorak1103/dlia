package anonymize

import (
	"regexp"
	"strings"
)

const secretPlaceholder = "<SECRET>"

const kvKey = `(?i)\b([a-z0-9_-]*(?:password|passwd|pwd|secret|token|` +
	`api[-_]?key|access[-_]?key|private[-_]?key|client[-_]?secret|secret[-_]?(?:access[-_]?)?key))\b`

// kvSep allows a quoted (or backslash-escaped quoted) key before the separator.
const kvSep = `(\\?["']?[ \t]*[:=][ \t]*)`

// kvPlain splits key, separator, whitespace and value so maskPlainKV can tell
// "password= user=bob" (empty value) from a real value.
var kvPlain = regexp.MustCompile(kvKey + `(\\?["']?[ \t]*[:=])([ \t]*)([^\s,;&"'\\][^\s,;&"']*)`)

// nextKV matches a would-be value that is really the next key=value pair. It is
// only consulted after an "=" separator and whitespace; "password: a=b" stays a
// secret (fail-safe).
var nextKV = regexp.MustCompile(`^[A-Za-z0-9_.-]+=[^=]`)

// secretRules are applied in order, before kvPlain. Replacements keep the key
// or prefix and only swap the value.
var secretRules = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`), secretPlaceholder},
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*`), secretPlaceholder},
	{regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), secretPlaceholder},
	{regexp.MustCompile(`(?i)(\bauthorization"?[ \t]*[:=][ \t]*"?)[^\r\n"']+`), "${1}" + secretPlaceholder},
	{regexp.MustCompile(`(?i)(\bbearer[ \t]+)[A-Za-z0-9._~+/=-]+`), "${1}" + secretPlaceholder},
	{regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^\s/@:"']*:[^\s/@"']*@`), "${1}" + secretPlaceholder + "@"},
	{regexp.MustCompile(kvKey + kvSep + `"(?:[^"\\]|\\.)+"`), `${1}${2}"` + secretPlaceholder + `"`},
	{regexp.MustCompile(kvKey + kvSep + `'[^']+'`), `${1}${2}'` + secretPlaceholder + `'`},
	{regexp.MustCompile(kvKey + kvSep + `\\"[^"\\]+\\"`), `${1}${2}\"` + secretPlaceholder + `\"`},
}

func maskSecrets(msg string) string {
	for _, r := range secretRules {
		msg = r.re.ReplaceAllString(msg, r.repl)
	}
	return kvPlain.ReplaceAllStringFunc(msg, maskPlainKV)
}

func maskPlainKV(match string) string {
	m := kvPlain.FindStringSubmatch(match)
	if m[3] != "" && strings.HasSuffix(m[2], "=") && nextKV.MatchString(m[4]) {
		// Empty value followed by another pair: keep it, but the pair may
		// itself be a secret key that this match consumed.
		return m[1] + m[2] + m[3] + kvPlain.ReplaceAllStringFunc(m[4], maskPlainKV)
	}
	return m[1] + m[2] + m[3] + secretPlaceholder
}
