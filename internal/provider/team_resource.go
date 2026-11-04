package provider

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/transmitdev/terraform-provider-transmit/internal/client"
)

type teamResource struct{ base }

type teamModel struct {
	ID              types.String `tfsdk:"id"`
	ConfigurationID types.String `tfsdk:"configuration_id"`
	Name            types.String `tfsdk:"name"`
	ResponderIDs    types.Set    `tfsdk:"responder_ids"`
}

func newTeamResource() resource.Resource { return &teamResource{} }

func (r *teamResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oncall_team"
}

func (r *teamResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A team of responders. Teams can own schedules and escalation policies; deleting a team is refused while it still owns one.",
		Attributes: map[string]schema.Attribute{
			"id":               idAttribute(),
			"configuration_id": configurationIDAttribute(),
			"name":             schema.StringAttribute{Required: true, Validators: nameValidators()},
			"responder_ids": schema.SetAttribute{ElementType: types.StringType, Optional: true, Computed: true, Default: setdefault.StaticValue(types.SetValueMust(types.StringType, nil)),
				Validators: []validator.Set{setvalidator.SizeAtMost(100), setvalidator.ValueStringsAre(idValidators()...)}, MarkdownDescription: "Members of the team."},
		},
	}
}

func (m *teamModel) set(ctx context.Context, c *client.Team, diags *diagnostics) {
	m.ID, m.ConfigurationID, m.Name = types.StringValue(c.ID), types.StringValue(c.ConfigurationID), types.StringValue(c.Name)
	m.ResponderIDs = setOfStrings(ctx, c.ResponderIDs, diags)
}

func (r *teamResource) write(ctx context.Context, method, url string, plan *teamModel, diags *diagnostics) bool {
	in := client.TeamInput{Name: plan.Name.ValueString(), ResponderIDs: stringsFromSet(ctx, plan.ResponderIDs, diags)}
	var out client.Team
	if err := r.client.Do(ctx, method, url, in, &out); err != nil {
		apiError(diags, "save team", err)
		return false
	}
	plan.set(ctx, &out, diags)
	pagingNotice(diags)
	return true
}

func (r *teamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan teamModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !r.write(ctx, http.MethodPost, client.OnCall(plan.ConfigurationID.ValueString(), "teams"), &plan, &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *teamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state teamModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out client.Team
	if err := r.client.Do(ctx, http.MethodGet, client.OnCall(state.ConfigurationID.ValueString(), "teams", state.ID.ValueString()), nil, &out); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		apiError(&resp.Diagnostics, "read team", err)
		return
	}
	state.set(ctx, &out, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *teamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state teamModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !r.write(ctx, http.MethodPut, client.OnCall(state.ConfigurationID.ValueString(), "teams", state.ID.ValueString()), &plan, &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *teamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state teamModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Do(ctx, http.MethodDelete, client.OnCall(state.ConfigurationID.ValueString(), "teams", state.ID.ValueString()), nil, nil); err != nil && !client.IsNotFound(err) {
		apiError(&resp.Diagnostics, "delete team", err)
	}
}

func (r *teamResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importInto(ctx, resp, req.ID, "configuration_id", "id")
}
