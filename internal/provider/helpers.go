package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/transmitdev/terraform-provider-transmit/internal/client"
)

// base holds the configured client for every resource and data source.
type base struct{ client *client.Client }

func (b *base) configure(providerData any, diags *diag.Diagnostics) {
	if providerData == nil {
		return
	}
	c, ok := providerData.(*client.Client)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("expected *client.Client, got %T", providerData))
		return
	}
	b.client = c
}

func (b *base) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	b.configure(req.ProviderData, &resp.Diagnostics)
}

func apiError(diags *diag.Diagnostics, action string, err error) {
	detail := err.Error()
	if apiErr, ok := err.(*client.APIError); ok && apiErr.Details != nil {
		for _, issue := range apiErr.Details.Issues {
			detail += fmt.Sprintf("\n  - %s: %s", issue.Path, issue.Message)
		}
	}
	diags.AddError("Transmit API: "+action, detail)
}

func pagingNotice(diags *diag.Diagnostics) {
	diags.AddWarning("On-call paging is now off for this configuration",
		"Transmit turns paging off after any on-call configuration change so that nobody is paged by a definition that has not been drilled. "+
			"Run a drill and re-activate paging in the dashboard (On-call → Paging) once the apply finishes.")
}

// importIDs splits "a/b/c" import identifiers.
func importIDs(raw string, names ...string) ([]string, error) {
	parts := strings.Split(raw, "/")
	if len(parts) != len(names) {
		return nil, fmt.Errorf("expected import identifier %q, got %q", strings.Join(names, "/"), raw)
	}
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("expected import identifier %q, got %q", strings.Join(names, "/"), raw)
		}
	}
	return parts, nil
}

func importInto(ctx context.Context, resp *resource.ImportStateResponse, raw string, names ...string) {
	parts, err := importIDs(raw, names...)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import identifier", err.Error())
		return
	}
	for i, name := range names {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(name), parts[i])...)
	}
}

func optionalString(value types.String) *string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	s := value.ValueString()
	return &s
}

func stringOrNull(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(*value)
}

func stringsFromList(ctx context.Context, list types.List, diags *diag.Diagnostics) []string {
	out := []string{}
	if list.IsNull() || list.IsUnknown() {
		return out
	}
	diags.Append(list.ElementsAs(ctx, &out, false)...)
	return out
}

func stringsFromSet(ctx context.Context, set types.Set, diags *diag.Diagnostics) []string {
	out := []string{}
	if set.IsNull() || set.IsUnknown() {
		return out
	}
	diags.Append(set.ElementsAs(ctx, &out, false)...)
	return out
}

func listOfStrings(ctx context.Context, values []string, diags *diag.Diagnostics) types.List {
	if values == nil {
		values = []string{}
	}
	list, d := types.ListValueFrom(ctx, types.StringType, values)
	diags.Append(d...)
	return list
}

func setOfStrings(ctx context.Context, values []string, diags *diag.Diagnostics) types.Set {
	if values == nil {
		values = []string{}
	}
	set, d := types.SetValueFrom(ctx, types.StringType, values)
	diags.Append(d...)
	return set
}

type diagnostics = diag.Diagnostics

func itoa(n int) string { return fmt.Sprintf("%d", n) }
