package endpointutil

import "strings"

func Resolve(baseURL, suffix string, complete bool) string {
	baseURL = strings.TrimSpace(baseURL)
	suffix = strings.TrimSpace(suffix)
	if !complete || suffix == "" {
		return strings.TrimRight(baseURL, "/")
	}

	trimmedBase := strings.TrimRight(baseURL, "/")
	trimmedSuffix := "/" + strings.TrimLeft(suffix, "/")
	if strings.HasSuffix(trimmedBase, trimmedSuffix) {
		return trimmedBase
	}
	return trimmedBase + trimmedSuffix
}
