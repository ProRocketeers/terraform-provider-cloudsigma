package cloudsigma

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudsigma/cloudsigma-sdk-go/cloudsigma"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/ProRocketeers/terraform-provider-cloudsigma/internal/tcloud"
)

// Config represents the configuration structure used to instantiate
// the Cloudsigma provider.
type Config struct {
	Token       string
	Username    string
	Password    string
	Location    string
	BaseURL     string
	OTPSecret   string
	Impersonate string

	context   context.Context
	userAgent string
}

// Client returns a new client for accessing CloudSigma.
func (c *Config) Client() (*cloudsigma.Client, error) {
	var creds cloudsigma.CredentialsProvider
	if len(c.Token) > 0 {
		creds = cloudsigma.NewTokenCredentialsProvider(c.Token)
		tflog.Info(c.context, "CloudSigma Client configured using access token", map[string]interface{}{
			"location": c.Location,
		})
	} else {
		creds = cloudsigma.NewUsernamePasswordCredentialsProvider(c.Username, c.Password)
		tflog.Info(c.context, "CloudSigma Client configured for user", map[string]interface{}{
			"location": c.Location,
			"username": c.Username,
		})
		log.Printf("[INFO] CloudSigma Client configured for user: %s, location: %s", c.Username, c.Location)
	}
	opts := []cloudsigma.ClientOption{cloudsigma.WithUserAgent(c.userAgent)}
	if c.BaseURL != "" {
		opts = append(opts, tcloud.BaseURLOption(c.BaseURL))
	} else {
		opts = append(opts, cloudsigma.WithLocation(c.Location))
	}
	if c.OTPSecret != "" {
		httpClient, err := tcloud.Login(c.context, tcloud.Endpoint(c.BaseURL, c.Location), c.Username, c.Password, c.OTPSecret, c.Impersonate, c.userAgent)
		if err != nil {
			return nil, err
		}
		opts = append(opts, cloudsigma.WithHTTPClient(httpClient))
	}

	return cloudsigma.NewClient(creds, opts...), nil
}

// loadAndValidate configures and returns a fully initialized CloudSigma SDK.
func (c *Config) loadAndValidate(ctx context.Context, terraformVersion string) {
	c.context = ctx

	providerVersion := fmt.Sprintf("terraform-provider-cloudsigma/%s", providerVersion)
	userAgent := fmt.Sprintf("Terraform/%s (https://www.terraform.io) %s", terraformVersion, providerVersion)
	c.userAgent = userAgent
}
