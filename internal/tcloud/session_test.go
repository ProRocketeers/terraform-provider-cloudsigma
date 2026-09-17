package tcloud

import (
	"testing"
	"time"

	"github.com/cloudsigma/cloudsigma-sdk-go/cloudsigma"
)

func TestTOTP(t *testing.T) {
	// RFC 6238 test vector: ASCII "12345678901234567890" in base32, T=59.
	got, err := TOTP("gezd gnbv gy3t qojq gezd gnbv gy3t qojq", time.Unix(59, 0))
	if err != nil {
		t.Fatal(err)
	}
	if got != "287082" {
		t.Errorf("TOTP = %q, want 287082", got)
	}
}

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
