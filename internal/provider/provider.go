package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/cloudsigma/cloudsigma-sdk-go/cloudsigma"

	csgo "github.com/ProRocketeers/cloudsigma-go"
	"github.com/ProRocketeers/terraform-provider-cloudsigma/internal/sdkopt"
)

const (
	defaultLocation = "zrh"
)

var _ provider.Provider = (*cloudSigmaProvider)(nil)

// cloudSigmaProvider defines the provider implementation.
type cloudSigmaProvider struct {
	// version is set to
	//  - the provider version on release
	//  - "dev" when the provider is built and ran locally
	//  - "testacc" when running acceptance tests
	version string
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &cloudSigmaProvider{
			version: version,
		}
	}
}

func (p *cloudSigmaProvider) Metadata(_ context.Context, _ provider.MetadataRequest, response *provider.MetadataResponse) {
	response.TypeName = "cloudsigma"
	response.Version = p.version
}

func (p *cloudSigmaProvider) Schema(_ context.Context, _ provider.SchemaRequest, response *provider.SchemaResponse) {
	response.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"base_url": schema.StringAttribute{
				Optional:    true,
				Description: "The API host and path, without a scheme. Overrides 'location'. Example: 'prg1.t-cloud.eu/api/2.0/'.",
			},
			"otp_secret": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "The base32 TOTP secret for accounts with enforced 2FA.",
			},
			"impersonate": schema.StringAttribute{
				Optional:    true,
				Description: "UUID of a user to impersonate after login. All operations then act as that user.",
			},
			"location": schema.StringAttribute{
				Optional:    true,
				Description: fmt.Sprintf("The location endpoint for CloudSigma. Default is '%s'.", defaultLocation),
			},
			"password": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "The CloudSigma password.",
			},
			"token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "The CloudSigma access token.",
			},
			"username": schema.StringAttribute{
				Optional:    true,
				Description: "The CloudSigma user email.",
			},
		},
	}
}

type providerModel struct {
	BaseURL     types.String `tfsdk:"base_url"`
	Location    types.String `tfsdk:"location"`
	OTPSecret   types.String `tfsdk:"otp_secret"`
	Impersonate types.String `tfsdk:"impersonate"`
	Password    types.String `tfsdk:"password"`
	Token       types.String `tfsdk:"token"`
	Username    types.String `tfsdk:"username"`
}

func (p *cloudSigmaProvider) Configure(ctx context.Context, request provider.ConfigureRequest, response *provider.ConfigureResponse) {
	var (
		config providerModel

		baseURL     string
		impersonate string
		location    string
		otpSecret   string
		password    string
		token       string
		username    string
	)

	diags := request.Config.Get(ctx, &config)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}

	// default values to environment variables, but override with config value if set
	baseURL = os.Getenv("CLOUDSIGMA_BASE_URL")
	otpSecret = os.Getenv("CLOUDSIGMA_OTP_SECRET")
	impersonate = os.Getenv("CLOUDSIGMA_IMPERSONATE")
	location = os.Getenv("CLOUDSIGMA_LOCATION")
	password = os.Getenv("CLOUDSIGMA_PASSWORD")
	token = os.Getenv("CLOUDSIGMA_TOKEN")
	username = os.Getenv("CLOUDSIGMA_USERNAME")

	if !config.Location.IsNull() {
		location = config.Location.ValueString()
	} else {
		if location == "" {
			tflog.Info(ctx, "Setting CloudSigma location to default value", map[string]interface{}{
				"location": defaultLocation,
			})
			location = defaultLocation
		}
	}
	if !config.BaseURL.IsNull() {
		baseURL = config.BaseURL.ValueString()
	}
	if !config.OTPSecret.IsNull() {
		otpSecret = config.OTPSecret.ValueString()
	}
	if !config.Impersonate.IsNull() {
		impersonate = config.Impersonate.ValueString()
	}
	if !config.Password.IsNull() {
		password = config.Password.ValueString()
	}
	if !config.Token.IsNull() {
		token = config.Token.ValueString()
	}
	if !config.Username.IsNull() {
		username = config.Username.ValueString()
	}

	// required if still unset
	if token != "" && (username != "" || password != "") {
		response.Diagnostics.AddError(
			"Ambiguous CloudSigma credentials",
			"Only one of the credential type must be set: [token] or [username,password]",
		)
	}
	if token == "" && username == "" && password == "" {
		response.Diagnostics.AddError(
			"Missing CloudSigma credentials",
			"Ensure that one of the credential types is set: [token] or [username,password]",
		)
	}
	if token == "" && username != "" && password == "" {
		response.Diagnostics.AddAttributeError(
			path.Root("password"),
			"Incomplete CloudSigma username/password credentials",
			`"password" must be set`,
		)
	}
	if token == "" && username == "" && password != "" {
		response.Diagnostics.AddAttributeError(
			path.Root("username"),
			"Incomplete CloudSigma username/password credentials",
			`"username" must be set`,
		)
	}
	if response.Diagnostics.HasError() {
		return
	}

	// build cloudsigma sdk client
	var creds cloudsigma.CredentialsProvider
	if token != "" {
		creds = cloudsigma.NewTokenCredentialsProvider(token)
		tflog.Info(ctx, "Configuring CloudSigma SDK client", map[string]interface{}{
			"credentials_provider": "token",
			"location":             location,
			"terraform_version":    request.TerraformVersion,
		})
	} else {
		creds = cloudsigma.NewUsernamePasswordCredentialsProvider(username, password)
		tflog.Info(ctx, "Configuring CloudSigma SDK client", map[string]interface{}{
			"credentials_provider": "username_and_password",
			"location":             location,
			"terraform_version":    request.TerraformVersion,
			"username":             username,
		})
	}
	opts := []cloudsigma.ClientOption{cloudsigma.WithUserAgent(p.userAgent())}
	if baseURL != "" {
		opts = append(opts, sdkopt.BaseURLOption(baseURL))
	} else {
		opts = append(opts, cloudsigma.WithLocation(location))
	}
	if otpSecret != "" {
		httpClient, err := csgo.Login(ctx, csgo.Endpoint(baseURL, location), username, password, otpSecret, impersonate, p.userAgent())
		if err != nil {
			response.Diagnostics.AddError("Cannot authenticate with CloudSigma", err.Error())
			return
		}
		opts = append(opts, cloudsigma.WithHTTPClient(httpClient))
	}
	client := cloudsigma.NewClient(creds, opts...)

	response.DataSourceData = client
	response.ResourceData = client
}

func (p *cloudSigmaProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewDriveDataSource,
		NewIPDataSource,
		NewLibraryDriveDataSource,
		NewLicenseDataSource,
		NewLocationDataSource,
		NewProfileDataSource,
		NewSubscriptionDataSource,
		NewTagDataSource,
		NewVLANDataSource,
	}
}

func (p *cloudSigmaProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewSnapshotResource,
		NewSSHKeyResource,
		NewTagResource,
	}
}

func (p *cloudSigmaProvider) userAgent() string {
	name := "terraform-provider-cloudsigma"
	comment := "https://registry.terraform.io/providers/cloudsigma/cloudsigma"

	return fmt.Sprintf("%s/%s (+%s)", name, p.version, comment)
}
