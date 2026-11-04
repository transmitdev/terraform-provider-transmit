package provider

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/puchesjr/terraform-provider-transmit/internal/client"
)

type escalationPolicyResource struct{ base }

type stepModel struct {
	DelaySeconds types.Int64  `tfsdk:"delay_seconds"`
	ResponderID  types.String `tfsdk:"responder_id"`
	ScheduleID   types.String `tfsdk:"schedule_id"`
}

type escalationPolicyModel struct {
	ID              types.String `tfsdk:"id"`
	ConfigurationID types.String `tfsdk:"configuration_id"`
	Name            types.String `tfsdk:"name"`
	OwnerTeamID     types.String `tfsdk:"owner_team_id"`
	Steps           []stepModel  `tfsdk:"step"`
}

func newEscalationPolicyResource() resource.Resource { return &escalationPolicyResource{} }

func (r *escalationPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oncall_escalation_policy"
}

func (r *escalationPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	recipient := func(other string) []validator.String {
		return append(idValidators(), stringvalidator.ExactlyOneOf(path.MatchRelative().AtParent().AtName(other)))
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Who to page, and when, until someone acknowledges. Alerts, integrations and heartbeats route to a configuration and an escalation policy.\n\n" +
			"Deleting is refused while heartbeats still point at the policy, and for a policy that has paged incidents (it is kept for their history); stop routing to it instead.",
		Attributes: map[string]schema.Attribute{
			"id":               idAttribute(),
			"configuration_id": configurationIDAttribute(),
			"name":             schema.StringAttribute{Required: true, Validators: nameValidators()},
			"owner_team_id":    schema.StringAttribute{Optional: true, Validators: idValidators(), MarkdownDescription: "Team that owns the policy."},
		},
		Blocks: map[string]schema.Block{
			"step": schema.ListNestedBlock{
				MarkdownDescription: "A step pages one responder, or whoever a schedule has on call, `delay_seconds` after the incident opens, unless it was acknowledged first.",
				Validators:          []validator.List{listvalidator.SizeBetween(1, 100), listvalidator.IsRequired()},
				NestedObject: schema.NestedBlockObject{Attributes: map[string]schema.Attribute{
					"delay_seconds": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0), Validators: []validator.Int64{int64validator.Between(0, 604800)}, MarkdownDescription: "Seconds after the incident opens. Defaults to `0`."},
					"responder_id":  schema.StringAttribute{Optional: true, Validators: recipient("schedule_id"), MarkdownDescription: "Page this responder. Set exactly one of `responder_id` or `schedule_id`."},
					"schedule_id":   schema.StringAttribute{Optional: true, Validators: recipient("responder_id"), MarkdownDescription: "Page whoever this schedule has on call."},
				}},
			},
		},
	}
}

func (m *escalationPolicyModel) input() client.EscalationPolicyInput {
	in := client.EscalationPolicyInput{Name: m.Name.ValueString(), OwnerTeamID: optionalString(m.OwnerTeamID), Steps: []client.StepInput{}}
	for _, step := range m.Steps {
		recipient := client.Recipient{Kind: "responder", ID: step.ResponderID.ValueString()}
		if !step.ScheduleID.IsNull() {
			recipient = client.Recipient{Kind: "schedule", ID: step.ScheduleID.ValueString()}
		}
		in.Steps = append(in.Steps, client.StepInput{DelaySeconds: step.DelaySeconds.ValueInt64(), Recipient: recipient})
	}
	return in
}

func (m *escalationPolicyModel) set(c *client.EscalationPolicy) {
	m.ID, m.ConfigurationID, m.Name, m.OwnerTeamID = types.StringValue(c.ID), types.StringValue(c.ConfigurationID), types.StringValue(c.Name), stringOrNull(c.OwnerTeamID)
	m.Steps = nil
	for _, step := range c.Steps {
		model := stepModel{DelaySeconds: types.Int64Value(step.DelaySeconds), ResponderID: types.StringNull(), ScheduleID: types.StringNull()}
		if step.Recipient.Kind == "schedule" {
			model.ScheduleID = types.StringValue(step.Recipient.ID)
		} else {
			model.ResponderID = types.StringValue(step.Recipient.ID)
		}
		m.Steps = append(m.Steps, model)
	}
}

func (r *escalationPolicyResource) save(ctx context.Context, method, url string, plan *escalationPolicyModel, diags *diagnostics) bool {
	var out client.EscalationPolicy
	if err := r.client.Do(ctx, method, url, plan.input(), &out); err != nil {
		apiError(diags, "save escalation policy", err)
		return false
	}
	plan.set(&out)
	pagingNotice(diags)
	return true
}

func (r *escalationPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan escalationPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !r.save(ctx, http.MethodPost, client.OnCall(plan.ConfigurationID.ValueString(), "escalation-policies"), &plan, &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *escalationPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state escalationPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out client.EscalationPolicy
	if err := r.client.Do(ctx, http.MethodGet, client.OnCall(state.ConfigurationID.ValueString(), "escalation-policies", state.ID.ValueString()), nil, &out); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		apiError(&resp.Diagnostics, "read escalation policy", err)
		return
	}
	state.set(&out)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *escalationPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state escalationPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !r.save(ctx, http.MethodPut, client.OnCall(state.ConfigurationID.ValueString(), "escalation-policies", state.ID.ValueString()), &plan, &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *escalationPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state escalationPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Do(ctx, http.MethodDelete, client.OnCall(state.ConfigurationID.ValueString(), "escalation-policies", state.ID.ValueString()), nil, nil); err != nil && !client.IsNotFound(err) {
		apiError(&resp.Diagnostics, "delete escalation policy", err)
	}
}

func (r *escalationPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importInto(ctx, resp, req.ID, "configuration_id", "id")
}
