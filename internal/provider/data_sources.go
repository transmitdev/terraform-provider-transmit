package provider

import (
	"context"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/puchesjr/terraform-provider-transmit/internal/client"
)

type dataBase struct{ base }

func (d *dataBase) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.configure(req.ProviderData, &resp.Diagnostics)
}

/* transmit_oncall_configuration */

type configurationDataSource struct{ dataBase }

func newConfigurationDataSource() datasource.DataSource { return &configurationDataSource{} }

func (d *configurationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oncall_configuration"
}

func (d *configurationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	oneOf := func(other string) []validator.String {
		return []validator.String{stringvalidator.ExactlyOneOf(path.MatchRoot(other))}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up an on-call configuration by id or name, such as one created by the Opsgenie importer.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Optional: true, Computed: true, Validators: oneOf("name")},
			"name":           schema.StringAttribute{Optional: true, Computed: true, Validators: oneOf("id"), MarkdownDescription: "Exact name. The lookup fails if no configuration or more than one has it."},
			"revision":       schema.Int64Attribute{Computed: true},
			"paging_enabled": schema.BoolAttribute{Computed: true},
		},
	}
}

func (d *configurationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config configurationModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var list client.List[client.Configuration]
	if err := d.client.Do(ctx, http.MethodGet, client.Path("on-call", "configurations"), nil, &list); err != nil {
		apiError(&resp.Diagnostics, "list configurations", err)
		return
	}
	var matches []client.Configuration
	for _, item := range list.Data {
		if (!config.ID.IsNull() && item.ID == config.ID.ValueString()) || (!config.Name.IsNull() && item.Name == config.Name.ValueString()) {
			matches = append(matches, item)
		}
	}
	if len(matches) != 1 {
		resp.Diagnostics.AddError("Configuration not found", "Expected exactly one matching on-call configuration; found "+itoa(len(matches))+".")
		return
	}
	config.set(&matches[0])
	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}

/* transmit_oncall_responder */

type responderDataSource struct{ dataBase }

func newResponderDataSource() datasource.DataSource { return &responderDataSource{} }

func (d *responderDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oncall_responder"
}

func (d *responderDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a responder by email, for configurations whose responders are managed outside Terraform.",
		Attributes: map[string]schema.Attribute{
			"configuration_id": schema.StringAttribute{Required: true, Validators: idValidators()},
			"email":            schema.StringAttribute{Required: true},
			"id":               schema.StringAttribute{Computed: true},
			"name":             schema.StringAttribute{Computed: true},
			"disabled":         schema.BoolAttribute{Computed: true},
			"contact_verified": schema.BoolAttribute{Computed: true},
		},
	}
}

func (d *responderDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config responderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var list client.List[client.Responder]
	if err := d.client.Do(ctx, http.MethodGet, client.OnCall(config.ConfigurationID.ValueString(), "responders"), nil, &list); err != nil {
		apiError(&resp.Diagnostics, "list responders", err)
		return
	}
	for _, item := range list.Data {
		if strings.EqualFold(item.Email, config.Email.ValueString()) {
			config.set(&item)
			resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
			return
		}
	}
	resp.Diagnostics.AddError("Responder not found", "No responder with email "+config.Email.ValueString()+" in this configuration.")
}
