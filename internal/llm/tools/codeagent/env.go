package codeagent

import "os"

// appendHeadlessEnv returns environment variables that force non-interactive/headless mode
// for external CLI tools. If base is nil, inherits from the current process environment.
func appendHeadlessEnv(base []string) []string {
	if base == nil {
		base = os.Environ()
	}
	return append(base,
		"CI=1",
		"TERM=dumb",
		"NO_COLOR=1",
	)
}
