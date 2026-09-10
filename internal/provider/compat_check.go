// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/vmware/terraform-provider-ssp/internal/compat"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

// warnOnVersionCompat looks up any compatibility.yaml constraints registered
// for resourceType and, only if at least one exists, fetches the live
// platform version and adds a warning diagnostic for any that aren't
// satisfied. With no constraints registered for a resource (the case for
// every resource today -- compatibility.yaml ships with an empty attribute
// list), this is a no-op and never makes a network call. Failure to fetch
// the live version is itself non-fatal: this check is advisory only and
// must never block a Create/Update that would otherwise succeed.
func warnOnVersionCompat(ctx context.Context, diags *diag.Diagnostics, c *client.Client, resourceType string) {
	constraints := compat.ForResource(resourceType)
	if len(constraints) == 0 {
		return
	}

	liveVersion, err := c.GetPlatformVersion(ctx)
	if err != nil {
		return
	}

	for _, constraint := range constraints {
		if msg := compat.Evaluate(constraint, liveVersion); msg != "" {
			diags.AddWarning("Version compatibility", msg)
		}
	}
}
