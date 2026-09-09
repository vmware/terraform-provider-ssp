// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

// securityContentConfigID is the fixed ID the real API assigns to this
// singleton config (per apis/ssp_public_apis.yaml's example response), not
// the generic "singleton" placeholder used by other singleton resources in
// this codebase.
const securityContentConfigID = "scs-config"

var _ resource.Resource = &SecurityContentConfigResource{}

func NewSecurityContentConfigResource() resource.Resource {
	return &SecurityContentConfigResource{}
}

// SecurityContentConfigResource manages the singleton security content
// service configuration (connectivity mode for cloud services).
type SecurityContentConfigResource struct {
	client *client.Client
}

// SecurityContentConfigResourceModel is the Terraform state model.
type SecurityContentConfigResourceModel struct {
	ID                         types.String `tfsdk:"id"`
	ConnectivityMode           types.String `tfsdk:"connectivity_mode"`
	ConnectivityModeChangeable types.Bool   `tfsdk:"connectivity_mode_changeable"`
}

func (r *SecurityContentConfigResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_content_config"
}

func (r *SecurityContentConfigResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the global Security Content service configuration " +
			"(connectivity mode for cloud services such as signature/feature downloads).\n\n" +
			"This is a singleton resource — only one configuration can exist per SSP instance, " +
			"with a fixed API-assigned ID of `scs-config`.\n" +
			"Use `terraform import ssp_security_content_config.this scs-config` to adopt an existing configuration.\n\n" +
			"Corresponds to `GET/PUT /ssp/security-content/config`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Always `scs-config` for this resource.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"connectivity_mode": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Connectivity mode controlling connectivity to cloud services " +
					"(e.g. `CONNECTED`, `DISCONNECTED`). If `connectivity_mode_changeable` is `false` " +
					"server-side, an attempted change is rejected with an error.",
			},
			"connectivity_mode_changeable": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether `connectivity_mode` can currently be changed. Read-only.",
			},
		},
	}
}

func (r *SecurityContentConfigResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, err := GetClientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected provider data type", err.Error())
		return
	}
	r.client = c
}

func (r *SecurityContentConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SecurityContentConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var current client.SecurityContentConfig
	if _, err := r.client.Get(ctx, "/ssp/security-content/config", &current); err != nil {
		resp.Diagnostics.AddError("Error reading current security content config", err.Error())
		return
	}

	payload := &client.SecurityContentConfig{
		Revision:         current.Revision,
		ConnectivityMode: data.ConnectivityMode.ValueString(),
	}

	var result client.SecurityContentConfig
	if status, err := r.client.Put(ctx, "/ssp/security-content/config", payload, &result); err != nil {
		resp.Diagnostics.AddError("Error creating security content config", formatSecurityContentConfigError(status, err))
		return
	}

	mapSecurityContentConfigToState(&result, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SecurityContentConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SecurityContentConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result client.SecurityContentConfig
	if _, err := r.client.Get(ctx, "/ssp/security-content/config", &result); err != nil {
		resp.Diagnostics.AddError("Error reading security content config", err.Error())
		return
	}

	mapSecurityContentConfigToState(&result, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SecurityContentConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data SecurityContentConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var current client.SecurityContentConfig
	if _, err := r.client.Get(ctx, "/ssp/security-content/config", &current); err != nil {
		resp.Diagnostics.AddError("Error reading current security content config", err.Error())
		return
	}

	payload := &client.SecurityContentConfig{
		Revision:         current.Revision,
		ConnectivityMode: data.ConnectivityMode.ValueString(),
	}

	var result client.SecurityContentConfig
	if status, err := r.client.Put(ctx, "/ssp/security-content/config", payload, &result); err != nil {
		resp.Diagnostics.AddError("Error updating security content config", formatSecurityContentConfigError(status, err))
		return
	}

	mapSecurityContentConfigToState(&result, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete removes the resource from state. The SSP API has no DELETE for
// security content config — it is a persistent platform setting.
func (r *SecurityContentConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
}

// formatSecurityContentConfigError adds context to the API's 409 Conflict,
// which is returned both for a stale/missing _revision and for an attempted
// connectivity_mode change while connectivity_mode_changeable is false.
func formatSecurityContentConfigError(status int, err error) string {
	if status == 409 {
		return fmt.Sprintf("%s (409 here means either the configuration was modified since it was last "+
			"read - retry the apply - or connectivity_mode is not currently changeable per the API's "+
			"connectivity_mode_changeable flag)", err.Error())
	}
	return err.Error()
}

func mapSecurityContentConfigToState(result *client.SecurityContentConfig, m *SecurityContentConfigResourceModel) {
	m.ID = types.StringValue(securityContentConfigID)
	m.ConnectivityMode = types.StringValue(result.ConnectivityMode)
	if result.ConnectivityModeChangeable != nil {
		m.ConnectivityModeChangeable = types.BoolValue(*result.ConnectivityModeChangeable)
	} else {
		m.ConnectivityModeChangeable = types.BoolNull()
	}
}
