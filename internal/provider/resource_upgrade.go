package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ resource.Resource = &UpgradeResource{}
var _ resource.ResourceWithImportState = &UpgradeResource{}

// upgradeSingletonID is the fixed Terraform ID for ssp_upgrade: the SSP
// public API exposes a single, cluster-wide upgrade workflow with no
// per-request identifier (POST /ssp/upgrade returns only a status_url),
// so the resource is modeled as a singleton against GET /ssp/upgrade/status.
const upgradeSingletonID = "upgrade"

func NewUpgradeResource() resource.Resource {
	return &UpgradeResource{}
}

// UpgradeResource manages SSP system upgrade execution.
type UpgradeResource struct {
	client *client.Client
}

// UpgradeResourceModel is the Terraform state model for upgrade execution.
type UpgradeResourceModel struct {
	ID             types.String `tfsdk:"id"`
	RunPrechecks   types.Bool   `tfsdk:"run_prechecks"`
	CurrentVersion types.String `tfsdk:"current_version"`
	TargetVersion  types.String `tfsdk:"target_version"`
	Status         types.String `tfsdk:"status"`
	Progress       types.Int64  `tfsdk:"progress"`
	CurrentStep    types.String `tfsdk:"current_step"`
}

func (r *UpgradeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_upgrade"
}

func (r *UpgradeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Triggers and tracks an in-place SSP cluster upgrade.\n\n" +
			"The SSP upgrade workflow has no user-selectable target version and no request\n" +
			"body: `POST /ssp/upgrade` is driven entirely by the `action` query parameter\n" +
			"(`PRECHECKS_ONLY`, `START`, `CONTINUE`, `RETRY`), and `target_version` /\n" +
			"`current_version` are read-only values reported by `GET /ssp/upgrade/status`\n" +
			"once an upgrade version has been staged in the depot.\n\n" +
			"On create, the resource deploys the `UPGRADE_COORDINATOR` feature, then:\n" +
			"  - if the last upgrade attempt is `FAILED`, issues `action=RETRY`;\n" +
			"  - else if `run_prechecks` is `true` (default), issues `action=PRECHECKS_ONLY`\n" +
			"    first and only proceeds to `action=CONTINUE` if pre-checks succeed;\n" +
			"  - else issues `action=START` directly.\n\n" +
			"On destroy, the `UPGRADE_COORDINATOR` feature is undeployed. There is only one\n" +
			"upgrade workflow per SSP cluster, so this resource is a singleton.\n\n" +
			"Corresponds to `POST /ssp/upgrade` and `GET /ssp/upgrade/status`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Fixed identifier (`upgrade`); the SSP API exposes a single cluster-wide upgrade workflow.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"run_prechecks": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether to run validation pre-checks before starting the upgrade. Ignored if a previous upgrade attempt failed (in which case the failed attempt is retried via `action=RETRY`).",
			},
			"current_version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "SSP version the cluster was running immediately before the upgrade, as reported by the API.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"target_version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "SSP version the cluster is being upgraded to, as reported by the API. Not user-settable — `POST /ssp/upgrade` has no version parameter.",
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Overall status of the upgrade operation (`NOT_STARTED`, `IN_PROGRESS`, `SUCCESS`, `FAILED`).",
			},
			"progress": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Percentage of upgrade steps that have completed successfully (0–100).",
			},
			"current_step": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Display name of the upgrade step currently executing (or the last step executed).",
			},
		},
	}
}

func (r *UpgradeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// deployUpgradeCoordinator fetches the current revision of UPGRADE_COORDINATOR
// and deploys it, waiting for deployment to complete. Mirrors the
// revision-fetch/error-propagation pattern used by FeatureResource.
func (r *UpgradeResource) deployUpgradeCoordinator(ctx context.Context) error {
	revision, err := r.getFeatureRevision(ctx)
	if err != nil {
		return fmt.Errorf("reading UPGRADE_COORDINATOR revision: %w", err)
	}

	if _, err := r.client.Put(ctx, "/ssp/lcm/features/UPGRADE_COORDINATOR", &client.FeatureDeployment{
		Revision: revision,
		Action:   "DEPLOY",
	}, nil); err != nil {
		return fmt.Errorf("deploying UPGRADE_COORDINATOR: %w", err)
	}

	deployStatus, err := r.client.WaitForFeatureDeployment(ctx, "UPGRADE_COORDINATOR")
	if err != nil {
		return fmt.Errorf("waiting for UPGRADE_COORDINATOR deployment: %w", err)
	}
	switch deployStatus.OverallStatus {
	case "DEPLOYMENT_FAILED", "PRECHECKS_FAILED":
		return fmt.Errorf("UPGRADE_COORDINATOR deployment failed: status=%s reason=%s",
			deployStatus.OverallStatus, deployStatus.DeploymentResults.Reason)
	}
	return nil
}

// undeployUpgradeCoordinator fetches the current revision of UPGRADE_COORDINATOR
// and undeploys it, waiting for the operation to complete.
func (r *UpgradeResource) undeployUpgradeCoordinator(ctx context.Context) error {
	revision, err := r.getFeatureRevision(ctx)
	if err != nil {
		return fmt.Errorf("reading UPGRADE_COORDINATOR revision: %w", err)
	}

	if _, err := r.client.Put(ctx, "/ssp/lcm/features/UPGRADE_COORDINATOR", &client.FeatureDeployment{
		Revision: revision,
		Action:   "UNDEPLOY",
	}, nil); err != nil {
		return fmt.Errorf("undeploying UPGRADE_COORDINATOR: %w", err)
	}

	undeployStatus, err := r.client.WaitForFeatureDeployment(ctx, "UPGRADE_COORDINATOR")
	if err != nil {
		return fmt.Errorf("waiting for UPGRADE_COORDINATOR undeployment: %w", err)
	}
	if undeployStatus.OverallStatus == "UNDEPLOYMENT_FAILED" {
		return fmt.Errorf("UPGRADE_COORDINATOR undeployment failed: reason=%s", undeployStatus.DeploymentResults.Reason)
	}
	return nil
}

// getFeatureRevision fetches the current _revision of UPGRADE_COORDINATOR.
func (r *UpgradeResource) getFeatureRevision(ctx context.Context) (int, error) {
	var fd client.FeatureDeployment
	if _, err := r.client.Get(ctx, "/ssp/lcm/features/UPGRADE_COORDINATOR", &fd); err != nil {
		return 0, fmt.Errorf("GET /ssp/lcm/features/UPGRADE_COORDINATOR: %w", err)
	}
	return fd.Revision, nil
}

// triggerUpgrade determines the correct action (RETRY / PRECHECKS_ONLY+CONTINUE / START)
// based on the current upgrade status and run_prechecks, then drives the upgrade to
// completion (or a reported pre-checks/upgrade failure).
func (r *UpgradeResource) triggerUpgrade(ctx context.Context, runPrechecks bool) (*client.UpgradeStatus, error) {
	var current client.UpgradeStatus
	if _, err := r.client.Get(ctx, "/ssp/upgrade/status", &current); err != nil {
		return nil, fmt.Errorf("reading current upgrade status: %w", err)
	}

	if current.OverallStatus == "FAILED" {
		if _, err := r.client.Post(ctx, "/ssp/upgrade?action=RETRY", nil, nil); err != nil {
			return nil, fmt.Errorf("retrying failed upgrade: %w", err)
		}
		return r.client.WaitForUpgradeComplete(ctx)
	}

	if runPrechecks {
		if _, err := r.client.Post(ctx, "/ssp/upgrade?action=PRECHECKS_ONLY", nil, nil); err != nil {
			return nil, fmt.Errorf("running upgrade pre-checks: %w", err)
		}
		precheckStatus, err := r.client.WaitForUpgradeStatus(ctx)
		if err != nil {
			return nil, fmt.Errorf("waiting for pre-checks to complete: %w", err)
		}
		if precheckStatus.OverallStatus == "FAILED" {
			return precheckStatus, fmt.Errorf("upgrade pre-checks failed")
		}

		if _, err := r.client.Post(ctx, "/ssp/upgrade?action=CONTINUE", nil, nil); err != nil {
			return nil, fmt.Errorf("continuing upgrade after successful pre-checks: %w", err)
		}
		return r.client.WaitForUpgradeComplete(ctx)
	}

	if _, err := r.client.Post(ctx, "/ssp/upgrade?action=START", nil, nil); err != nil {
		return nil, fmt.Errorf("starting upgrade: %w", err)
	}
	return r.client.WaitForUpgradeComplete(ctx)
}

func (r *UpgradeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data UpgradeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	lcmMu.Lock()
	defer lcmMu.Unlock()

	err := r.deployUpgradeCoordinator(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error deploying UPGRADE_COORDINATOR", err.Error())
		return
	}

	// UPGRADE_COORDINATOR is now genuinely deployed server-side; record the
	// resource's existence before attempting the long-running upgrade itself,
	// so a triggerUpgrade failure below doesn't leave Terraform with no record
	// of it (which would cause the next apply to try to deploy it again).
	data.ID = types.StringValue(upgradeSingletonID)

	status, err := r.triggerUpgrade(ctx, data.RunPrechecks.ValueBool())
	if status != nil {
		mapUpgradeStatusToState(status, &data)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if err != nil {
		resp.Diagnostics.AddError("Upgrade operation failed", err.Error())
	}
}

func (r *UpgradeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), upgradeSingletonID)...)
}

func (r *UpgradeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data UpgradeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var status client.UpgradeStatus
	httpStatus, err := r.client.Get(ctx, "/ssp/upgrade/status", &status)
	if httpStatus == 404 {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading upgrade status", err.Error())
		return
	}

	mapUpgradeStatusToState(&status, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update retries a FAILED upgrade (action=RETRY). Other attribute changes
// (e.g. run_prechecks) have no effect once an upgrade has been triggered.
func (r *UpgradeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data UpgradeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var current client.UpgradeStatus
	if _, err := r.client.Get(ctx, "/ssp/upgrade/status", &current); err != nil {
		resp.Diagnostics.AddError("Error reading current upgrade status", err.Error())
		return
	}

	if current.OverallStatus != "FAILED" {
		mapUpgradeStatusToState(&current, &data)
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}

	lcmMu.Lock()
	defer lcmMu.Unlock()

	if _, err := r.client.Post(ctx, "/ssp/upgrade?action=RETRY", nil, nil); err != nil {
		resp.Diagnostics.AddError("Error retrying failed upgrade", err.Error())
		return
	}

	status, err := r.client.WaitForUpgradeComplete(ctx)
	if status != nil {
		mapUpgradeStatusToState(status, &data)
	} else {
		data.ID = types.StringValue(upgradeSingletonID)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if err != nil {
		resp.Diagnostics.AddError("Upgrade retry failed", err.Error())
	}
}

// Delete undeploys the UPGRADE_COORDINATOR feature, per the recommended
// post-upgrade workflow (PUT /ssp/lcm/features/UPGRADE_COORDINATOR?action=UNDEPLOY).
func (r *UpgradeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	lcmMu.Lock()
	defer lcmMu.Unlock()

	if err := r.undeployUpgradeCoordinator(ctx); err != nil {
		resp.Diagnostics.AddError("Error undeploying UPGRADE_COORDINATOR", err.Error())
	}
}

func mapUpgradeStatusToState(s *client.UpgradeStatus, m *UpgradeResourceModel) {
	m.ID = types.StringValue(upgradeSingletonID)
	m.Status = types.StringValue(s.OverallStatus)
	m.CurrentVersion = types.StringValue(s.CurrentVersion)
	m.TargetVersion = types.StringValue(s.TargetVersion)
	m.Progress = types.Int64Value(int64(s.StepProgressPercent()))
	m.CurrentStep = types.StringValue(s.CurrentStepName())
}
