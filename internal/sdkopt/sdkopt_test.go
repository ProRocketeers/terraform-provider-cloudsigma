package sdkopt

import (
	"testing"

	"github.com/cloudsigma/cloudsigma-sdk-go/cloudsigma"
)

func TestBaseURLOption(t *testing.T) {
	client := cloudsigma.NewClient(
		cloudsigma.NewTokenCredentialsProvider("x"),
		BaseURLOption("prg1.t-cloud.eu/api/2.0/"),
	)

	request, err := client.NewRequest("GET", "servers/", nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://prg1.t-cloud.eu/api/2.0/servers/"; request.URL.String() != want {
		t.Errorf("URL = %q, want %q", request.URL, want)
	}
}
