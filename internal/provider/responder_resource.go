package provider

import (
	"context"
	"net/http"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/transmitdev/terraform-provider-transmit/internal/client"
)

type responderResource struct{ base }

type responderModel struct {
	ID              types.String `tfsdk:"id"`
	ConfigurationID types.String `tfsdk:"configuration_id"`
	Name            types.String `tfsdk:"name"`
	Email           types.String `tfsdk:"email"`
	Disabled        types.Bool   `tfsdk:"disabled"`
	ContactVerified types.Bool   `tfsdk:"contact_verified"`
}

func newResponderResource() resource.Resource { return &responderResource{} }

func (r *responderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oncall_responder"
}

func configurationIDAttribute() schema.StringAttribute {
	return schema.StringAttribute{Required: true, Validators: idValidators(), PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "The on-call configuration this belongs to. Changing it replaces the resource."}
}

func idAttribute() schema.StringAttribute {
	return schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}}
}

func (r *responderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A person who can be paged. Transmit pages by email until the responder verifies phone contacts in the dashboard; changing `email` or disabling the responder revokes that verification.\n\nDeleting is refused while a team, schedule or escalation policy still names the responder, so reference responders by id and Terraform orders the deletes for you.",
		Attributes: map[string]schema.Attribute{
			"id":               idAttribute(),
			"configuration_id": configurationIDAttribute(),
			"name":             schema.StringAttribute{Required: true, Validators: nameValidators()},
			"email": schema.StringAttribute{Required: true, MarkdownDescription: "Lowercase email address, unique within the configuration.",
				Validators: []validatorString{stringvalidator.LengthBetween(3, 254), stringvalidator.RegexMatches(regexp.MustCompile(`^[^A-Z\s]+@[^A-Z\s]+$`), "must be a lowercase email address")}},
			"disabled":         schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "A disabled responder is never paged. Defaults to `false`."},
			"contact_verified": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the responder has verified a phone contact for SMS and voice paging."},
		},
	}
}

func (m *responderModel) set(c *client.Responder) {
	m.ID, m.ConfigurationID, m.Name, m.Email = types.StringValue(c.ID), types.StringValue(c.ConfigurationID), types.StringValue(c.Name), types.StringValue(c.Email)
	m.Disabled, m.ContactVerified = types.BoolValue(c.Disabled), types.BoolValue(c.ContactVerified)
}

func (m *responderModel) input() client.ResponderInput {
	return client.ResponderInput{Name: m.Name.ValueString(), Email: m.Email.ValueString(), Disabled: m.Disabled.ValueBool()}
}

func (r *responderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan responderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out client.Responder
	if err := r.client.Do(ctx, http.MethodPost, client.OnCall(plan.ConfigurationID.ValueString(), "responders"), plan.input(), &out); err != nil {
		apiError(&resp.Diagnostics, "create responder", err)
		return
	}
	plan.set(&out)
	pagingNotice(&resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *responderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state responderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out client.Responder
	if err := r.client.Do(ctx, http.MethodGet, client.OnCall(state.ConfigurationID.ValueString(), "responders", state.ID.ValueString()), nil, &out); err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		apiError(&resp.Diagnostics, "read responder", err)
		return
	}
	state.set(&out)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *responderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state responderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out client.Responder
	if err := r.client.Do(ctx, http.MethodPut, client.OnCall(state.ConfigurationID.ValueString(), "responders", state.ID.ValueString()), plan.input(), &out); err != nil {
		apiError(&resp.Diagnostics, "update responder", err)
		return
	}
	plan.set(&out)
	pagingNotice(&resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *responderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state responderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Do(ctx, http.MethodDelete, client.OnCall(state.ConfigurationID.ValueString(), "responders", state.ID.ValueString()), nil, nil); err != nil && !client.IsNotFound(err) {
		apiError(&resp.Diagnostics, "delete responder", err)
	}
}

func (r *responderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importInto(ctx, resp, req.ID, "configuration_id", "id")
}
