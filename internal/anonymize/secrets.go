package anonymize

import "regexp"

const secretPlaceholder = "<SECRET>"

const kvKey = `(?i)\b([a-z0-9_-]*(?:password|passwd|pwd|secret|token|api_key|apikey))\b`

const kvSep = `("?[ \t]*[:=][ \t]*)`

// secretRules are applied in order. Replacements keep the key or prefix and
// only swap the value, so a value that is already <SECRET> is left as is.
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
	{regexp.MustCompile(kvKey + kvSep + `(["'])[^"']+["']`), "${1}${2}${3}" + secretPlaceholder + "${3}"},
	{regexp.MustCompile(kvKey + kvSep + `[^\s,;&"']+`), "${1}${2}" + secretPlaceholder},
}

func maskSecrets(msg string) string {
	for _, r := range secretRules {
		msg = r.re.ReplaceAllString(msg, r.repl)
	}
	return msg
}
