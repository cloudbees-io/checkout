package git

import (
	"strings"
)

// envEntriesToMap is a helper method that takes a slice of KEY=VAL environment entries and turns them into a map
func envEntriesToMap(entries []string) map[string]string {
	r := make(map[string]string, len(entries))
	for _, e := range entries {
		s := strings.SplitN(e, "=", 2)
		r[s[0]] = s[1]
	}
	return r
}

// envMapToEntries is a helper method that takes a map and converts it into a slice of KEY=VAL environment
func envMapToEntries(entries map[string]string) []string {
	r := make([]string, len(entries))
	for k, v := range entries {
		r = append(r, k+"="+v)
	}
	return r
}
