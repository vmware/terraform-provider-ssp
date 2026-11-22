// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ resource.Resource = &UpgradeCoordinatorConfigResource{}
var _ resource.ResourceWithImportState = &UpgradeCoordinatorConfigResource{}

func NewUpgradeCoordinatorConfigResource() resource.Resource {
	return &UpgradeCoordinatorConfigResource{}
}

// UpgradeCoordinatorConfigResource manages the singleton pre-deployment
// configuration of the Upgrade Coordinator, which holds the upgrade target
// version.
type UpgradeCoordinatorConfigResource struct {
	client *client.Client
}

type UpgradeCoordinatorConfigResourceModel struct {
	ID            types.String `tfsdk:"id"`
	TargetVersion types.String `tfsdk:"target_version"`
	Revision      types.Int64  `tfsdk:"revision"`
	ConfigStatus  types.String `tfsdk:"config_status"`
	ConfigMessage types.String `tfsdk:"config_message"`
}

func (r *UpgradeCoordinatorConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_upgrade_coordinator_config"
}

func (r *UpgradeCoordinatorConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the pre-upgrade configuration of the **Upgrade Coordinator**: the SSP version to upgrade to.\n\n" +
			"The Upgrade Coordinator cannot be deployed until a target version is set, so apply this resource " +
			"before `ssp_upgrade` and order them with `depends_on = [ssp_upgrade_coordinator_config.<name>]`. " +
			"Valid target versions are listed by the `ssp_upgrade_available_versions` data source.\n\n" +
			"The backend only accepts a change while the Upgrade Coordinator is **not** deployed " +
			"(\"upgrade coordinator is already deployed\" otherwise). `ssp_upgrade` deploys it on create and " +
			"undeploys it on destroy, so set the version before creating `ssp_upgrade`, and destroy `ssp_upgrade` " +
			"before changing it.\n\n" +
			"This is a singleton resource — use `terraform import ssp_upgrade_coordinator_config.this singleton` to adopt an existing configuration. " +
			"There is no delete API, so `Delete` only removes the resource from Terraform state and the configuration stays on the cluster.\n\n" +
			"Corresponds to `GET/PUT /ssp/lcm/upgrade-coordinator/config` and `GET /ssp/lcm/upgrade-coordinator/config/status`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Always `singleton` for this resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"target_version": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "SSP version to upgrade to, for example `5.2.2-0.0-105352618`. Must be one of the versions returned by `ssp_upgrade_available_versions`.",
			},
			"revision": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Revision number maintained by the SSP backend.",
			},
			"config_status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Configuration status from the platform, for example `DEPLOYMENT_REQUIRED` or `CONFIGURATION_APPLIED`.",
			},
			"config_message": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Informational message accompanying `config_status`.",
			},
		},
	}
}

func (r *UpgradeCoordinatorConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// readUpgradeCoordinatorConfig fills data from GET .../config and
// GET .../config/status. The status shape ({status, message}) is the same as
// the sibling config resources', so the shared client type is reused.
func (r *UpgradeCoordinatorConfigResource) readUpgradeCoordinatorConfig(ctx context.Context, data *UpgradeCoordinatorConfigResourceModel) (int, error) {
	var cfg client.UpgradeCoordinatorConfiguration
	httpStatus, err := r.client.Get(ctx, "/ssp/lcm/upgrade-coordinator/config", &cfg)
	if err != nil {
		return httpStatus, err
	}
	data.ID = types.StringValue("singleton")
	data.TargetVersion = types.StringValue(cfg.TargetVersion)
	data.Revision = types.Int64Value(int64(cfg.Revision))

	var status client.CloudConnectorConfigurationStatus
	if _, err := r.client.Get(ctx, "/ssp/lcm/upgrade-coordinator/config/status", &status); err != nil {
		return httpStatus, fmt.Errorf("reading upgrade coordinator configuration status: %w", err)
	}
	data.ConfigStatus = types.StringValue(status.Status)
	data.ConfigMessage = types.StringValue(status.Message)
	return httpStatus, nil
}

// apply sets the target version using the backend's optimistic locking: the
// current revision is fetched fresh and sent with the PUT, and the result is
// re-read to confirm the value was actually stored (a stale revision makes the
// backend ignore the PUT).
func (r *UpgradeCoordinatorConfigResource) apply(ctx context.Context, data *UpgradeCoordinatorConfigResourceModel) error {
	// The revision-fetch-then-PUT sequence races with any other SSP LCM
	// lifecycle action (feature/site/upgrade/backup/restore); serialize with them.
	lcmMu.Lock()
	defer lcmMu.Unlock()

	want := data.TargetVersion.ValueString()

	var current client.UpgradeCoordinatorConfiguration
	if _, err := r.client.Get(ctx, "/ssp/lcm/upgrade-coordinator/config", &current); err != nil {
		return fmt.Errorf("reading current upgrade coordinator configuration: %w", err)
	}

	if _, err := r.client.Put(ctx, "/ssp/lcm/upgrade-coordinator/config", client.UpgradeCoordinatorConfiguration{
		Revision:      current.Revision,
		TargetVersion: want,
	}, nil); err != nil {
		return fmt.Errorf("setting upgrade coordinator configuration: %w", err)
	}

	if _, err := r.readUpgradeCoordinatorConfig(ctx, data); err != nil {
		return err
	}
	if got := data.TargetVersion.ValueString(); got != want {
		return fmt.Errorf("upgrade coordinator target_version is %q after update, expected %q", got, want)
	}
	return nil
}

func (r *UpgradeCoordinatorConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data UpgradeCoordinatorConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.apply(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error configuring upgrade coordinator", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UpgradeCoordinatorConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data UpgradeCoordinatorConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	httpStatus, err := r.readUpgradeCoordinatorConfig(ctx, &data)
	if httpStatus == 404 {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading upgrade coordinator configuration", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UpgradeCoordinatorConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data UpgradeCoordinatorConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.apply(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error updating upgrade coordinator configuration", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UpgradeCoordinatorConfigResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Singleton configuration: no delete API. The config stays on the cluster.
}

func (r *UpgradeCoordinatorConfigResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), "singleton")...)
}
