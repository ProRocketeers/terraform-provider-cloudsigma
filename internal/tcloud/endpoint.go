package tcloud

import (
	"strings"

	"github.com/cloudsigma/cloudsigma-sdk-go/cloudsigma"
)

// Endpoint returns the API host and path (no scheme) for the given base URL,
// falling back to the CloudSigma location subdomain.
func Endpoint(baseURL, location string) string {
	if baseURL == "" {
		return location + ".cloudsigma.com/api/2.0/"
	}
	baseURL = strings.TrimPrefix(strings.TrimPrefix(baseURL, "https://"), "http://")
	return strings.TrimSuffix(baseURL, "/") + "/"
}

// BaseURLOption points the SDK at an arbitrary API host.
//
// ponytail: the SDK hardcodes "https://%s.cloudsigma.com/api/2.0/" and exposes
// no base-URL option, so the trailing "#" parks the leftover of that format
// string in the URL fragment, which ResolveReference drops when the SDK builds
// request URLs. Upgrade path: a WithBaseURL option upstream. Guarded by
// TestBaseURLOption.
func BaseURLOption(baseURL string) cloudsigma.ClientOption {
	return cloudsigma.WithLocation(Endpoint(baseURL, "") + "#")
}
