package provider

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/puchesjr/terraform-provider-transmit/internal/client"
)

type overrideResource struct{ base }

type overrideModel struct {
	ID              types.String `tfsdk:"id"`
	ConfigurationID types.String `tfsdk:"configuration_id"`
	ScheduleID      types.String `tfsdk:"schedule_id"`
	StartsAt        types.String `tfsdk:"starts_at"`
	EndsAt          types.String `tfsdk:"ends_at"`
	ResponderID     types.String `tfsdk:"responder_id"`
	RotationIDs     types.Set    `tfsdk:"rotation_ids"`
}

func newOverrideResource() resource.Resource { return &overrideResource{} }

func (r *overrideResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oncall_schedule_override"
}

func (r *overrideResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Puts one responder on call in place of a schedule's rotations for a period, such as covering a vacation. A schedule holds at most 1000 overrides, so delete past ones.",
		Attributes: map[string]schema.Attribute{
			"id":               idAttribute(),
			"configuration_id": configurationIDAttribute(),
			"schedule_id":      schema.StringAttribute{Required: true, Validators: idValidators(), PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "The schedule to override. Changing it replaces the override."},
			"starts_at":        schema.StringAttribute{Required: true, Validators: timestampValidators(), MarkdownDescription: "RFC 3339 with an explicit offset."},
			"ends_at":          schema.StringAttribute{Required: true, Validators: timestampValidators(), MarkdownDescription: "RFC 3339 with an explicit offset; after `starts_at`."},
			"responder_id":     schema.StringAttribute{Required: true, Validators: idValidators(), MarkdownDescription: "Who is on call instead."},
			"rotation_ids": schema.SetAttribute{ElementType: types.StringType, Optional: true, Computed: true,
				PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
				Validators:    []validator.Set{setvalidator.SizeBetween(1, 100), setvalidator.ValueStringsAre(idValidators()...)},
				MarkdownDescription: "Rotations the override replaces, usually from `transmit_oncall_schedule.<name>.rotation_ids`. Omit to cover every rotation the schedule had when the override was created."},
		},
	}
}

func (m *overrideModel) set(ctx context.Context, c *client.Override, diags *diagnostics) {
	m.ID, m.ConfigurationID, m.ScheduleID = types.StringValue(c.ID), types.StringValue(c.ConfigurationID), types.StringValue(c.ScheduleID)
	m.StartsAt, m.EndsAt, m.ResponderID = types.StringValue(c.StartsAt), types.StringValue(c.EndsAt), types.StringValue(c.ResponderID)
	m.RotationIDs = setOfStrings(ctx, c.RotationIDs, diags)
}

func (m *overrideModel) input(ctx context.Context, diags *diagnostics) client.OverrideInput {
	in := client.OverrideInput{StartsAt: m.StartsAt.ValueString(), EndsAt: m.EndsAt.ValueString(), ResponderID: m.ResponderID.ValueString()}
	if !m.RotationIDs.IsNull() && !m.RotationIDs.IsUnknown() {
		in.RotationIDs = stringsFromSet(ctx, m.RotationIDs, diags)
	}
	return in
}

func (m *overrideModel) url(segments ...string) string {
	return client.OnCall(m.ConfigurationID.ValueString(), append([]string{"schedules", m.ScheduleID.ValueString(), "overrides"}, segments...)...)
}

func (r *overrideResource) save(ctx context.Context, method, url string, plan *overrideModel, diags *diagnostics) bool {
	in := plan.input(ctx, diags)
	var out client.Override
	if err := r.client.Do(ctx, method, url, in, &out); err != nil {
		apiError(diags, "save schedule override", err)
		return false
	}
	plan.set(ctx, &out, diags)
	pagingNotice(diags)
	return true
}

func (r *overrideResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan overrideModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !r.save(ctx, http.MethodPost, plan.url(), &plan, &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *overrideResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state overrideModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out client.Override
	if err := r.client.Do(ctx, http.MethodGet, state.url(state.ID.ValueString()), nil, &out); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		apiError(&resp.Diagnostics, "read schedule override", err)
		return
	}
	state.set(ctx, &out, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *overrideResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state overrideModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !r.save(ctx, http.MethodPut, state.url(state.ID.ValueString()), &plan, &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *overrideResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state overrideModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Do(ctx, http.MethodDelete, state.url(state.ID.ValueString()), nil, nil); err != nil && !client.IsNotFound(err) {
		apiError(&resp.Diagnostics, "delete schedule override", err)
	}
}

func (r *overrideResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importInto(ctx, resp, req.ID, "configuration_id", "schedule_id", "id")
}
