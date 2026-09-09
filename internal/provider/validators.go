// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// noTrailingSlashValidator rejects a string value ending in "/". This exists
// for Required (non-Computed) path-like attributes whose resource
// state-mapping logic trims a trailing slash from the API response (e.g.
// ssp_backup_config's backup_location): a plan modifier cannot be used to
// silently normalize such a value instead, because the terraform-plugin-
// framework requires a non-Computed attribute's final state to exactly equal
// its planned (== configured) value — silently rewriting it produces
// "Provider produced invalid plan". Failing fast at plan/validate time with
// a clear message is both correct and more helpful than either outcome.
type noTrailingSlashValidator struct{}

func noTrailingSlash() validator.String {
	return noTrailingSlashValidator{}
}

func (v noTrailingSlashValidator) Description(_ context.Context) string {
	return "value must not end in a trailing slash"
}

func (v noTrailingSlashValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v noTrailingSlashValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsUnknown() || req.ConfigValue.IsNull() {
		return
	}
	value := req.ConfigValue.ValueString()
	if strings.HasSuffix(value, "/") {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Trailing Slash Not Allowed",
			fmt.Sprintf("%q must not end in a trailing slash (the API strips it, which would otherwise make "+
				"every plan after this one show a permanent diff); use %q instead.",
				value, strings.TrimRight(value, "/")),
		)
	}
}
