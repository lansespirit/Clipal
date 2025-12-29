package selfupdate

import (
	"fmt"
	"strconv"
	"strings"
)

// compareVersions compares semver-ish tags like "v0.1.2" or "0.1.2".
// Returns -1 if a<b, 0 if equal, 1 if a>b, ok=false if unparsable.
func compareVersions(a, b string) (cmp int, ok bool) {
	a = strings.TrimSpace(strings.TrimPrefix(a, "v"))
	b = strings.TrimSpace(strings.TrimPrefix(b, "v"))
	if a == "" || b == "" {
		return 0, false
	}
	ap, err := parseVersionParts(a)
	if err != nil {
		return 0, false
	}
	bp, err := parseVersionParts(b)
	if err != nil {
		return 0, false
	}
	for i := 0; i < 3; i++ {
		if ap[i] < bp[i] {
			return -1, true
		}
		if ap[i] > bp[i] {
			return 1, true
		}
	}
	return 0, true
}

func parseVersionParts(v string) ([3]int, error) {
	var out [3]int
	parts := strings.SplitN(v, "-", 2)
	core := parts[0]
	fields := strings.Split(core, ".")
	if len(fields) < 1 || len(fields) > 3 {
		return out, fmt.Errorf("invalid version %q", v)
	}
	for i := 0; i < 3; i++ {
		if i >= len(fields) {
			out[i] = 0
			continue
		}
		n, err := strconv.Atoi(fields[i])
		if err != nil || n < 0 {
			return out, fmt.Errorf("invalid version %q", v)
		}
		out[i] = n
	}
	return out, nil
}
