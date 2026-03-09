package provider

import (
	"context"
	"os"

	"github.com/massive-hosting/go-hosting"
	"github.com/massive-hosting/terraform-provider-hosting/internal/resources"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = &hostingProvider{}

type hostingProvider struct {
	version string
}

type hostingProviderModel struct {
	APIURL     types.String `tfsdk:"api_url"`
	Token      types.String `tfsdk:"token"`
	CustomerID types.String `tfsdk:"customer_id"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &hostingProvider{version: version}
	}
}

func (p *hostingProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "hosting"
	resp.Version = p.version
}

func (p *hostingProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage hosting resources (webapps, databases, DNS, containers, etc.) via the control panel API.",
		Attributes: map[string]schema.Attribute{
			"api_url": schema.StringAttribute{
				Description: "Base URL of the control panel API. Can also be set via HOSTING_API_URL environment variable.",
				Optional:    true,
			},
			"token": schema.StringAttribute{
				Description: "Personal Access Token for API authentication. Can also be set via HOSTING_TOKEN environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
			"customer_id": schema.StringAttribute{
				Description: "Default customer ID for resource creation. Can also be set via HOSTING_CUSTOMER_ID environment variable.",
				Optional:    true,
			},
		},
	}
}

func (p *hostingProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config hostingProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiURL := envOrValue(config.APIURL, "HOSTING_API_URL", "")
	token := envOrValue(config.Token, "HOSTING_TOKEN", "")
	customerID := envOrValue(config.CustomerID, "HOSTING_CUSTOMER_ID", "")

	if apiURL == "" {
		resp.Diagnostics.AddError("Missing API URL", "Set api_url in provider config or HOSTING_API_URL environment variable.")
		return
	}
	if token == "" {
		resp.Diagnostics.AddError("Missing Token", "Set token in provider config or HOSTING_TOKEN environment variable.")
		return
	}

	c := hosting.New(apiURL, token)

	resp.DataSourceData = &resources.ProviderData{Client: c, CustomerID: customerID}
	resp.ResourceData = &resources.ProviderData{Client: c, CustomerID: customerID}
}

func (p *hostingProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resources.NewWebapp,
		resources.NewDatabase,
		resources.NewDatabaseUser,
		resources.NewValkey,
		resources.NewValkeyUser,
		resources.NewFQDN,
		resources.NewDNSZone,
		resources.NewDNSRecord,
		resources.NewS3Bucket,
		resources.NewS3AccessKey,
		resources.NewEmailAccount,
		resources.NewEmailAlias,
		resources.NewEmailForward,
		resources.NewWireGuardPeer,
		resources.NewSSHKey,
		resources.NewContainer,
		resources.NewEgressRule,
		resources.NewWebappEnvVars,
		resources.NewContainerEnvVars,
		resources.NewWebappDaemon,
		resources.NewWebappCronJob,
		resources.NewPreviewConfig,
	}
}

func (p *hostingProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

func envOrValue(v types.String, envKey, fallback string) string {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueString()
	}
	if env := os.Getenv(envKey); env != "" {
		return env
	}
	return fallback
}
