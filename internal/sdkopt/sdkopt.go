package sdkopt

import (
	"github.com/cloudsigma/cloudsigma-sdk-go/cloudsigma"

	csgo "github.com/ProRocketeers/cloudsigma-go"
)

// BaseURLOption points the SDK at an arbitrary API host.
//
// ponytail: the SDK hardcodes "https://%s.cloudsigma.com/api/2.0/" and exposes
// no base-URL option, so the trailing "#" parks the leftover of that format
// string in the URL fragment, which ResolveReference drops when the SDK builds
// request URLs. Upgrade path: a WithBaseURL option upstream. Guarded by
// TestBaseURLOption.
func BaseURLOption(baseURL string) cloudsigma.ClientOption {
	return cloudsigma.WithLocation(csgo.Endpoint(baseURL, "") + "#")
}
