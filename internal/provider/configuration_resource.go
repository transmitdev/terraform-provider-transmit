package provider

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/transmitdev/terraform-provider-transmit/internal/client"
)

type configurationResource struct{ base }

type configurationModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Revision      types.Int64  `tfsdk:"revision"`
	PagingEnabled types.Bool   `tfsdk:"paging_enabled"`
}

func newConfigurationResource() resource.Resource { return &configurationResource{} }

func (r *configurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oncall_configuration"
}

func (r *configurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An on-call configuration: the container for responders, teams, schedules and escalation policies that incidents route to.\n\n" +
			"~> **Destroying this resource only removes it from Terraform state.** Transmit keeps the configuration so incident history and integration URLs stay valid; delete its contents with Terraform, then remove it in the dashboard if you need to.",
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, MarkdownDescription: "Configuration id."},
			"name": schema.StringAttribute{Required: true, Validators: nameValidators(), MarkdownDescription: "Display name."},
			"revision": schema.Int64Attribute{Computed: true, MarkdownDescription: "Revision at the last read. It moves on every on-call change, including ones made by other resources."},
			"paging_enabled": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether paging is active. Any on-call change turns it off until a drill and re-activation in the dashboard."},
		},
	}
}

func (m *configurationModel) set(c *client.Configuration) {
	m.ID, m.Name, m.Revision, m.PagingEnabled = types.StringValue(c.ID), types.StringValue(c.Name), types.Int64Value(c.Revision), types.BoolValue(c.PagingEnabled)
}

func (r *configurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan configurationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out client.Configuration
	if err := r.client.Do(ctx, http.MethodPost, client.Path("on-call", "configurations"), map[string]string{"name": plan.Name.ValueString()}, &out); err != nil {
		apiError(&resp.Diagnostics, "create configuration", err)
		return
	}
	plan.set(&out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *configurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state configurationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out client.Configuration
	if err := r.client.Do(ctx, http.MethodGet, client.OnCall(state.ID.ValueString()), nil, &out); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		apiError(&resp.Diagnostics, "read configuration", err)
		return
	}
	state.set(&out)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *configurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state configurationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out client.Configuration
	if err := r.client.Do(ctx, http.MethodPatch, client.OnCall(state.ID.ValueString()), map[string]string{"name": plan.Name.ValueString()}, &out); err != nil {
		apiError(&resp.Diagnostics, "rename configuration", err)
		return
	}
	plan.set(&out)
	pagingNotice(&resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *configurationResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning("Configuration kept in Transmit", "Transmit does not delete on-call configurations through the API. It was removed from Terraform state only.")
}

func (r *configurationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ModifyPlan keeps revision and paging_enabled from showing as changes on every plan.
func (r *configurationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var plan, state configurationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !plan.Name.Equal(state.Name) {
		return
	}
	plan.Revision, plan.PagingEnabled = state.Revision, state.PagingEnabled
	resp.Diagnostics.Append(resp.Plan.Set(ctx, plan)...)
}
