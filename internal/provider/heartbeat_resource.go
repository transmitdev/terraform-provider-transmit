package provider

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/puchesjr/terraform-provider-transmit/internal/client"
)

type heartbeatResource struct{ base }

type heartbeatModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	ConfigurationID    types.String `tfsdk:"configuration_id"`
	EscalationPolicyID types.String `tfsdk:"escalation_policy_id"`
	IntervalSeconds    types.Int64  `tfsdk:"interval_seconds"`
	GraceSeconds       types.Int64  `tfsdk:"grace_seconds"`
	PingURL            types.String `tfsdk:"ping_url"`
}

func newHeartbeatResource() resource.Resource { return &heartbeatResource{} }

func (r *heartbeatResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_heartbeat"
}

func (r *heartbeatResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Pages an escalation policy when a job stops checking in. The heartbeat waits for its first ping, then opens one incident when `interval_seconds + grace_seconds` pass without another; the next ping resolves it.\n\n" +
			"Missed-heartbeat incidents are raised as the API key that created the heartbeat, so revoking that key stops its pages. A test key creates a test heartbeat. Changing the interval moves an armed deadline from the last ping, so an apply cannot hide a heartbeat that is already late.",
		Attributes: map[string]schema.Attribute{
			"id":                   idAttribute(),
			"name":                 schema.StringAttribute{Required: true, Validators: []validator.String{stringvalidator.LengthBetween(1, 100)}, MarkdownDescription: "Unique per workspace and test/live mode."},
			"configuration_id":     configurationIDAttribute(),
			"escalation_policy_id": schema.StringAttribute{Required: true, Validators: idValidators(), MarkdownDescription: "Policy in the same configuration to page on a miss."},
			"interval_seconds":     schema.Int64Attribute{Required: true, Validators: []validator.Int64{int64validator.Between(60, 604800)}, MarkdownDescription: "Expected time between pings."},
			"grace_seconds":        schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(300), Validators: []validator.Int64{int64validator.Between(0, 86400)}, MarkdownDescription: "Extra slack before a miss. Defaults to `300`."},
			"ping_url":             schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, MarkdownDescription: "POST here with an API key holding `incidents:ingest` to check in."},
		},
	}
}

func (m *heartbeatModel) set(c *client.Heartbeat) {
	m.ID, m.Name, m.ConfigurationID, m.EscalationPolicyID = types.StringValue(c.ID), types.StringValue(c.Name), types.StringValue(c.ConfigurationID), types.StringValue(c.EscalationPolicyID)
	m.IntervalSeconds, m.GraceSeconds, m.PingURL = types.Int64Value(c.IntervalSeconds), types.Int64Value(c.GraceSeconds), types.StringValue(c.PingURL)
}

func (r *heartbeatResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan heartbeatModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := client.HeartbeatCreate{Name: plan.Name.ValueString(), ConfigurationID: plan.ConfigurationID.ValueString(), EscalationPolicyID: plan.EscalationPolicyID.ValueString(), IntervalSeconds: plan.IntervalSeconds.ValueInt64(), GraceSeconds: plan.GraceSeconds.ValueInt64()}
	var out client.Heartbeat
	if err := r.client.Do(ctx, http.MethodPost, client.Path("heartbeats"), in, &out); err != nil {
		apiError(&resp.Diagnostics, "create heartbeat", err)
		return
	}
	plan.set(&out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *heartbeatResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state heartbeatModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out client.Heartbeat
	if err := r.client.Do(ctx, http.MethodGet, client.Path("heartbeats", state.ID.ValueString()), nil, &out); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		apiError(&resp.Diagnostics, "read heartbeat", err)
		return
	}
	state.set(&out)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *heartbeatResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state heartbeatModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := client.HeartbeatUpdate{Name: plan.Name.ValueString(), EscalationPolicyID: plan.EscalationPolicyID.ValueString(), IntervalSeconds: plan.IntervalSeconds.ValueInt64(), GraceSeconds: plan.GraceSeconds.ValueInt64()}
	var out client.Heartbeat
	if err := r.client.Do(ctx, http.MethodPut, client.Path("heartbeats", state.ID.ValueString()), in, &out); err != nil {
		apiError(&resp.Diagnostics, "update heartbeat", err)
		return
	}
	plan.set(&out)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *heartbeatResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state heartbeatModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Do(ctx, http.MethodDelete, client.Path("heartbeats", state.ID.ValueString()), nil, nil); err != nil && !client.IsNotFound(err) {
		apiError(&resp.Diagnostics, "delete heartbeat", err)
	}
}

func (r *heartbeatResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
