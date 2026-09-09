// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ resource.Resource = &NdrConfigResource{}
var _ resource.ResourceWithImportState = &NdrConfigResource{}

func NewNdrConfigResource() resource.Resource {
	return &NdrConfigResource{}
}

// NdrConfigResource manages the singleton NDR pre-deployment configuration.
type NdrConfigResource struct {
	client *client.Client
}

type NdrConfigResourceModel struct {
	ID            types.String `tfsdk:"id"`
	DataSharing   types.Bool   `tfsdk:"data_sharing"`
	Revision      types.Int64  `tfsdk:"revision"`
	ConfigStatus  types.String `tfsdk:"config_status"`
	ConfigMessage types.String `tfsdk:"config_message"`
}

func (r *NdrConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ndr_config"
}

func (r *NdrConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages pre-deployment configuration for **NSX NDR** (Network Detection & Response) on the SSP platform.\n\n" +
			"Controls cloud threat intelligence data sharing settings.\n\n" +
			"This is a singleton resource — use `terraform import ssp_ndr_config.this singleton` to adopt an existing configuration.\n\n" +
			"Corresponds to `GET/PUT /ssp/lcm/ndr/config`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Always `singleton` for this resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"data_sharing": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Flag controlling whether NDR shares detection data to cloud for threat intelligence.",
			},
			"revision": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Revision number maintained by the SSP backend.",
			},
			"config_status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Configuration status from the platform. One of `NOT_CONFIGURED`, `DEPLOYMENT_REQUIRED`, or `CONFIGURATION_APPLIED`.",
			},
			"config_message": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Informational message accompanying `config_status`.",
			},
		},
	}
}

// readNdrConfigStatus fetches GET /ssp/lcm/ndr/config/status and populates
// config_status/config_message. Reuses client.CloudConnectorConfigurationStatus
// since that sibling config resource's status response has the identical
// {status, message} shape.
func (r *NdrConfigResource) readNdrConfigStatus(ctx context.Context, data *NdrConfigResourceModel) error {
	var status client.CloudConnectorConfigurationStatus
	if _, err := r.client.Get(ctx, "/ssp/lcm/ndr/config/status", &status); err != nil {
		return err
	}
	data.ConfigStatus = types.StringValue(status.Status)
	data.ConfigMessage = types.StringValue(status.Message)
	return nil
}

func (r *NdrConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *NdrConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NdrConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The revision-fetch-then-PUT sequence below races with any other SSP LCM
	// lifecycle action (feature/site/upgrade/backup/restore); serialize with them.
	lcmMu.Lock()
	defer lcmMu.Unlock()

	// Fetch the current revision first to satisfy the backend's optimistic-locking
	// check on PUT — without it the PUT is silently ignored and the value never changes.
	var current client.NdrConfiguration
	if _, err := r.client.Get(ctx, "/ssp/lcm/ndr/config", &current); err != nil {
		resp.Diagnostics.AddError("Error reading current NDR configuration", err.Error())
		return
	}

	payload := client.NdrConfiguration{
		Revision:    current.Revision,
		DataSharing: data.DataSharing.ValueBool(),
	}

	if _, err := r.client.Put(ctx, "/ssp/lcm/ndr/config", payload, nil); err != nil {
		resp.Diagnostics.AddError("Error setting NDR configuration", err.Error())
		return
	}

	// The PUT may be accepted (202) before the change is reflected on GET; re-read
	// the config rather than trusting the PUT response body, mirroring the proven
	// pattern in resource_cloud_connector_config.go.
	var out client.NdrConfiguration
	if _, err := r.client.Get(ctx, "/ssp/lcm/ndr/config", &out); err != nil {
		resp.Diagnostics.AddError("Error reading NDR configuration after update", err.Error())
		return
	}

	data.ID = types.StringValue("singleton")
	data.DataSharing = types.BoolValue(out.DataSharing)
	data.Revision = types.Int64Value(int64(out.Revision))
	if err := r.readNdrConfigStatus(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading NDR configuration status", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NdrConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data NdrConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var out client.NdrConfiguration
	status, err := r.client.Get(ctx, "/ssp/lcm/ndr/config", &out)
	if status == 404 {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading NDR configuration", err.Error())
		return
	}

	data.ID = types.StringValue("singleton")
	data.DataSharing = types.BoolValue(out.DataSharing)
	data.Revision = types.Int64Value(int64(out.Revision))
	if err := r.readNdrConfigStatus(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading NDR configuration status", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NdrConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan NdrConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Serialize with other SSP LCM lifecycle actions (feature/site/upgrade/backup/restore).
	lcmMu.Lock()
	defer lcmMu.Unlock()

	// Fetch the current revision fresh (like Create does) rather than trusting the
	// last-known Terraform state revision, which can be stale if the config changed
	// out-of-band since the last Read -- a stale revision causes the backend's
	// optimistic-locking check to reject this PUT.
	var current client.NdrConfiguration
	if _, err := r.client.Get(ctx, "/ssp/lcm/ndr/config", &current); err != nil {
		resp.Diagnostics.AddError("Error reading current NDR configuration", err.Error())
		return
	}

	payload := client.NdrConfiguration{
		Revision:    current.Revision,
		DataSharing: plan.DataSharing.ValueBool(),
	}

	if _, err := r.client.Put(ctx, "/ssp/lcm/ndr/config", payload, nil); err != nil {
		resp.Diagnostics.AddError("Error updating NDR configuration", err.Error())
		return
	}

	var out client.NdrConfiguration
	if _, err := r.client.Get(ctx, "/ssp/lcm/ndr/config", &out); err != nil {
		resp.Diagnostics.AddError("Error reading NDR configuration after update", err.Error())
		return
	}

	plan.ID = types.StringValue("singleton")
	plan.DataSharing = types.BoolValue(out.DataSharing)
	plan.Revision = types.Int64Value(int64(out.Revision))
	if err := r.readNdrConfigStatus(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading NDR configuration status", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NdrConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Singleton configuration remains on backend with default settings.
}

func (r *NdrConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), "singleton")...)
}
