package provider

import (
	"context"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"

	"github.com/transmitdev/terraform-provider-transmit/internal/client"
)

type scheduleResource struct{ base }

type rotationModel struct {
	Name         types.String `tfsdk:"name"`
	Cadence      types.String `tfsdk:"cadence"`
	Interval     types.Int64  `tfsdk:"interval"`
	StartsAt     types.String `tfsdk:"starts_at"`
	EndsAt       types.String `tfsdk:"ends_at"`
	ResponderIDs types.List   `tfsdk:"responder_ids"`
}

type scheduleModel struct {
	ID              types.String    `tfsdk:"id"`
	ConfigurationID types.String    `tfsdk:"configuration_id"`
	Name            types.String    `tfsdk:"name"`
	Timezone        types.String    `tfsdk:"timezone"`
	Enabled         types.Bool      `tfsdk:"enabled"`
	OwnerTeamID     types.String    `tfsdk:"owner_team_id"`
	Rotations       []rotationModel `tfsdk:"rotation"`
	RotationIDs     types.Map       `tfsdk:"rotation_ids"`
}

func newScheduleResource() resource.Resource { return &scheduleResource{} }

func (r *scheduleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oncall_schedule"
}

func (r *scheduleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A schedule: one or more rotations that decide who is on call. Escalation policy steps can page whoever a schedule has on call.\n\n" +
			"A rotation keeps its id while its `name` is unchanged, so overrides that cover it survive edits. Renaming or removing a rotation that an override still covers is refused; delete those overrides first. " +
			"Overrides are managed with `transmit_oncall_schedule_override` and are not affected by this resource.",
		Attributes: map[string]schema.Attribute{
			"id":               idAttribute(),
			"configuration_id": configurationIDAttribute(),
			"name":             schema.StringAttribute{Required: true, Validators: nameValidators()},
			"timezone":         schema.StringAttribute{Required: true, Validators: []validator.String{stringvalidator.LengthBetween(1, 100)}, MarkdownDescription: "IANA time zone the rotations hand off in, such as `America/New_York`."},
			"enabled":          schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "A disabled schedule pages nobody. Defaults to `true`."},
			"owner_team_id":    schema.StringAttribute{Optional: true, Validators: idValidators(), MarkdownDescription: "Team that owns the schedule."},
			"rotation_ids": schema.MapAttribute{ElementType: types.StringType, Computed: true,
				MarkdownDescription: "Rotation ids by rotation name, for `transmit_oncall_schedule_override.rotation_ids`."},
		},
		Blocks: map[string]schema.Block{
			"rotation": schema.ListNestedBlock{
				MarkdownDescription: "A rotation. Names must be unique within the schedule (case-insensitive). Order is preserved.",
				Validators:          []validator.List{listvalidator.SizeBetween(1, 100), listvalidator.IsRequired()},
				NestedObject: schema.NestedBlockObject{Attributes: map[string]schema.Attribute{
					"name":     schema.StringAttribute{Required: true, Validators: nameValidators()},
					"cadence":  schema.StringAttribute{Required: true, Validators: []validator.String{stringvalidator.OneOf("hourly", "daily", "weekly")}, MarkdownDescription: "`hourly`, `daily` or `weekly`."},
					"interval": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(1), Validators: []validator.Int64{int64validator.Between(1, 10000)}, MarkdownDescription: "Hand off every `interval` cadence units. Defaults to `1`."},
					"starts_at": schema.StringAttribute{Required: true, Validators: timestampValidators(), MarkdownDescription: "When the first shift starts, as RFC 3339 with an explicit offset."},
					"ends_at":   schema.StringAttribute{Optional: true, Validators: timestampValidators(), MarkdownDescription: "When the rotation stops. Omit for no end."},
					"responder_ids": schema.ListAttribute{ElementType: types.StringType, Required: true,
						Validators: []validator.List{listvalidator.SizeBetween(1, 100), listvalidator.ValueStringsAre(idValidators()...)}, MarkdownDescription: "Responders in hand-off order."},
				}},
			},
		},
	}
}

func (m *scheduleModel) input(ctx context.Context, diags *diagnostics) client.ScheduleInput {
	in := client.ScheduleInput{Name: m.Name.ValueString(), Timezone: m.Timezone.ValueString(), Enabled: m.Enabled.ValueBool(), OwnerTeamID: optionalString(m.OwnerTeamID)}
	for _, rotation := range m.Rotations {
		in.Rotations = append(in.Rotations, client.RotationInput{
			Name: rotation.Name.ValueString(), Cadence: rotation.Cadence.ValueString(), Interval: rotation.Interval.ValueInt64(),
			StartsAt: rotation.StartsAt.ValueString(), EndsAt: optionalString(rotation.EndsAt), ResponderIDs: stringsFromList(ctx, rotation.ResponderIDs, diags),
		})
	}
	return in
}

func (m *scheduleModel) set(ctx context.Context, c *client.Schedule, diags *diagnostics) {
	m.ID, m.ConfigurationID, m.Name, m.Timezone = types.StringValue(c.ID), types.StringValue(c.ConfigurationID), types.StringValue(c.Name), types.StringValue(c.Timezone)
	m.Enabled, m.OwnerTeamID = types.BoolValue(c.Enabled), stringOrNull(c.OwnerTeamID)
	m.Rotations = nil
	ids := map[string]attr.Value{}
	for _, rotation := range c.Rotations {
		m.Rotations = append(m.Rotations, rotationModel{
			Name: types.StringValue(rotation.Name), Cadence: types.StringValue(rotation.Cadence), Interval: types.Int64Value(rotation.Interval),
			StartsAt: types.StringValue(rotation.StartsAt), EndsAt: stringOrNull(rotation.EndsAt), ResponderIDs: listOfStrings(ctx, rotation.ResponderIDs, diags),
		})
		ids[rotation.Name] = types.StringValue(rotation.ID)
	}
	m.RotationIDs = types.MapValueMust(types.StringType, ids)
}

func (r *scheduleResource) write(ctx context.Context, method, url string, plan *scheduleModel, diags *diagnostics) bool {
	in := plan.input(ctx, diags)
	if diags.HasError() {
		return false
	}
	var out client.Schedule
	if err := r.client.Do(ctx, method, url, in, &out); err != nil {
		apiError(diags, "save schedule", err)
		return false
	}
	plan.set(ctx, &out, diags)
	pagingNotice(diags)
	return true
}

func (r *scheduleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan scheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !r.write(ctx, http.MethodPost, client.OnCall(plan.ConfigurationID.ValueString(), "schedules"), &plan, &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *scheduleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state scheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out client.Schedule
	if err := r.client.Do(ctx, http.MethodGet, client.OnCall(state.ConfigurationID.ValueString(), "schedules", state.ID.ValueString()), nil, &out); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		apiError(&resp.Diagnostics, "read schedule", err)
		return
	}
	state.set(ctx, &out, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *scheduleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state scheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !r.write(ctx, http.MethodPut, client.OnCall(state.ConfigurationID.ValueString(), "schedules", state.ID.ValueString()), &plan, &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *scheduleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state scheduleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Do(ctx, http.MethodDelete, client.OnCall(state.ConfigurationID.ValueString(), "schedules", state.ID.ValueString()), nil, nil); err != nil && !client.IsNotFound(err) {
		apiError(&resp.Diagnostics, "delete schedule", err)
	}
}

func (r *scheduleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importInto(ctx, resp, req.ID, "configuration_id", "id")
}

// ModifyPlan predicts rotation_ids: the API keeps a rotation's id while its name (case-insensitively) is unchanged.
func (r *scheduleResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var plan scheduleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	known := map[string]string{}
	if !req.State.Raw.IsNull() {
		var state scheduleModel
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
		for name, id := range state.RotationIDs.Elements() {
			if s, ok := id.(types.String); ok {
				known[strings.ToLower(name)] = s.ValueString()
			}
		}
	}
	ids := map[string]attr.Value{}
	for _, rotation := range plan.Rotations {
		if rotation.Name.IsUnknown() {
			plan.RotationIDs = types.MapUnknown(types.StringType)
			resp.Diagnostics.Append(resp.Plan.Set(ctx, plan)...)
			return
		}
		if id, ok := known[strings.ToLower(rotation.Name.ValueString())]; ok {
			ids[rotation.Name.ValueString()] = types.StringValue(id)
		} else {
			ids[rotation.Name.ValueString()] = types.StringUnknown()
		}
	}
	plan.RotationIDs = types.MapValueMust(types.StringType, ids)
	resp.Diagnostics.Append(resp.Plan.Set(ctx, plan)...)
}
