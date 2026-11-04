// Package provider implements the Transmit Terraform provider.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/transmitdev/terraform-provider-transmit/internal/client"
)

var _ provider.Provider = (*transmitProvider)(nil)

type transmitProvider struct{ version string }

type providerModel struct {
	APIKey  types.String `tfsdk:"api_key"`
	BaseURL types.String `tfsdk:"base_url"`
}

// New returns a provider factory.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &transmitProvider{version: version} }
}

func (p *transmitProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "transmit"
	resp.Version = p.version
}

func (p *transmitProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage Transmit on-call (responders, teams, schedules, overrides, escalation policies and heartbeats) as code. " +
			"Use a **live** API key created with the `terraform` scope preset (`on_call:read`, `on_call:write`, `incidents:read`, `incidents:write`, `incidents:ingest`).\n\n" +
			"Any change to on-call configuration turns paging off for that configuration until an administrator runs a drill and re-activates paging in the dashboard, exactly as a dashboard edit does.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				MarkdownDescription: "Transmit API key (`tr_v2_…`). Defaults to the `TRANSMIT_API_KEY` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"base_url": schema.StringAttribute{
				MarkdownDescription: "API base URL. Defaults to `TRANSMIT_API_URL`, then `https://api.transmit.dev`.",
				Optional:            true,
			},
		},
	}
}

func (p *transmitProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiKey := os.Getenv("TRANSMIT_API_KEY")
	if !config.APIKey.IsNull() && !config.APIKey.IsUnknown() {
		apiKey = config.APIKey.ValueString()
	}
	baseURL := os.Getenv("TRANSMIT_API_URL")
	if !config.BaseURL.IsNull() && !config.BaseURL.IsUnknown() {
		baseURL = config.BaseURL.ValueString()
	}
	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(path.Root("api_key"), "Missing Transmit API key", "Set api_key in the provider block or the TRANSMIT_API_KEY environment variable.")
		return
	}
	c := client.New(baseURL, apiKey, "terraform-provider-transmit/"+p.version)
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *transmitProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newConfigurationResource,
		newResponderResource,
		newTeamResource,
		newScheduleResource,
		newOverrideResource,
		newEscalationPolicyResource,
		newHeartbeatResource,
	}
}

func (p *transmitProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newConfigurationDataSource,
		newResponderDataSource,
	}
}
