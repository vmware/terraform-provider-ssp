// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

// sspFeatureEnum lists every real value of the SspFeature enum
// (apis/ssp_public_apis.yaml), used to validate the `feature` attribute.
var sspFeatureEnum = []string{
	"MALWARE_PREVENTION",
	"INTELLIGENCE",
	"NDR",
	"BAREMETAL_SECURITY",
	"RULE_ANALYSIS",
	"AI_ASSISTANT_PLATFORM",
	"AI_ASSISTANT_THREAT_DEFENSE",
	"MALWARE_ANALYSIS_VC",
	"SPAC",
	"NETWORK_TRAFFIC_ANALYSIS",
	"UPGRADE_COORDINATOR",
	"CLOUD_CONNECTOR",
	"METRICS",
}

// lcmMu serialises all SSP LCM lifecycle actions within a single Terraform
// apply/destroy: the SSP platform only supports one concurrent lifecycle
// action at a time across ssp_feature (deploy/undeploy) and ssp_site
// (onboard/reconnect/offboard). Without this, Terraform's default resource
// parallelism can submit two lifecycle actions concurrently (e.g. an
// ssp_feature undeploy and an ssp_site offboard with no depends_on between
// them); the second one is silently queued and never progresses, which
// shows up as a resource hanging indefinitely until the conflicting action
// completes.
var lcmMu sync.Mutex

var _ resource.Resource = &FeatureResource{}
var _ resource.ResourceWithImportState = &FeatureResource{}

func NewFeatureResource() resource.Resource {
	return &FeatureResource{}
}

// FeatureResource manages the deployment lifecycle of a single SSP feature.
type FeatureResource struct {
	client *client.Client
}

// FeatureResourceModel is the Terraform state model.
type FeatureResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Feature          types.String `tfsdk:"feature"`
	ForceUndeploy    types.Bool   `tfsdk:"force_undeploy"`
	OverallStatus    types.String `tfsdk:"overall_status"`
	OverallProgress  types.Int64  `tfsdk:"overall_progress"`
	PrecheckStatus   types.String `tfsdk:"precheck_status"`
	DeploymentStatus types.String `tfsdk:"deployment_status"`
}

func (r *FeatureResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_feature"
}

func (r *FeatureResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Deploys (or undeploys) a vDefend feature on the SSP platform.\n\n" +
			"The resource runs pre-checks automatically before deploying. Only one LCM\n" +
			"lifecycle action (feature deploy/undeploy or site onboard/offboard) can run\n" +
			"at a time; concurrent `ssp_feature`/`ssp_site` resources within the same\n" +
			"`terraform apply` are serialised by the provider.\n\n" +
			"Supported features: `MALWARE_PREVENTION`, `INTELLIGENCE`, `NDR`, `BAREMETAL_SECURITY`,\n" +
			"`RULE_ANALYSIS`, `AI_ASSISTANT_PLATFORM`, `AI_ASSISTANT_THREAT_DEFENSE`,\n" +
			"`MALWARE_ANALYSIS_VC`, `SPAC`, `NETWORK_TRAFFIC_ANALYSIS`, `UPGRADE_COORDINATOR`,\n" +
			"`CLOUD_CONNECTOR`, `METRICS`.\n\n" +
			"Corresponds to `GET/PUT /ssp/lcm/features/{feature}` and\n" +
			"`GET /ssp/lcm/features/{feature}/status`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"feature": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the feature to deploy (case-sensitive).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:          []validator.String{stringvalidator.OneOf(sspFeatureEnum...)},
			},
			"force_undeploy": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				MarkdownDescription: "When `true`, `Delete` issues `action=FORCE_UNDEPLOY` instead of `action=UNDEPLOY`, " +
					"overriding a dependent-feature block that would otherwise fail a plain undeploy. Defaults to `false`.",
			},
			"overall_status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current overall deployment status of the feature.",
			},
			"overall_progress": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Overall deployment progress as a percentage (0–100).",
			},
			"precheck_status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Overall status of the most recent pre-check run.",
			},
			"deployment_status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Detailed deployment status from the deployment results object.",
			},
		},
	}
}

func (r *FeatureResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create runs RUN_PRECHECK → DEPLOY and polls until the feature is deployed.
func (r *FeatureResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data FeatureResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	feature := data.Feature.ValueString()

	lcmMu.Lock()
	defer lcmMu.Unlock()

	// ── Step 1: RUN_PRECHECK ──────────────────────────────────────────────────
	revision, err := r.getRevision(ctx, feature)
	if err != nil {
		resp.Diagnostics.AddError("Error reading feature revision", err.Error())
		return
	}

	_, err = r.client.Put(ctx, "/ssp/lcm/features/"+feature, &client.FeatureDeployment{
		Revision: revision,
		Action:   "RUN_PRECHECK",
	}, nil)
	if err != nil {
		resp.Diagnostics.AddError("Error triggering pre-checks for feature "+feature, err.Error())
		return
	}

	precheckStatus, err := r.client.WaitForFeaturePrechecks(ctx, feature)
	if err != nil {
		resp.Diagnostics.AddError("Error waiting for pre-checks", err.Error())
		return
	}

	if precheckStatus.PrecheckResults.OverallStatus == "FAILED" {
		var failedChecks []string
		for _, pc := range precheckStatus.PrecheckResults.Prechecks {
			if pc.Status == "FAILED" {
				failedChecks = append(failedChecks, fmt.Sprintf("  - %s: %s", pc.Name, pc.Reason))
			}
		}
		resp.Diagnostics.AddError(
			"Pre-checks failed for feature "+feature,
			"The following pre-checks failed:\n"+strings.Join(failedChecks, "\n"),
		)
		return
	}

	// ── Step 2: DEPLOY ────────────────────────────────────────────────────────
	revision, err = r.getRevision(ctx, feature)
	if err != nil {
		resp.Diagnostics.AddError("Error reading feature revision before deploy", err.Error())
		return
	}

	_, err = r.client.Put(ctx, "/ssp/lcm/features/"+feature, &client.FeatureDeployment{
		Revision: revision,
		Action:   "DEPLOY",
	}, nil)
	if err != nil {
		resp.Diagnostics.AddError("Error triggering deploy for feature "+feature, err.Error())
		return
	}

	deployStatus, err := r.client.WaitForFeatureDeployment(ctx, feature)
	if err != nil {
		resp.Diagnostics.AddError("Error waiting for feature deployment", err.Error())
		return
	}

	switch deployStatus.OverallStatus {
	case "DEPLOYMENT_FAILED", "PRECHECKS_FAILED":
		resp.Diagnostics.AddError(
			"Deployment failed for feature "+feature,
			fmt.Sprintf("Status: %s. Reason: %s",
				deployStatus.OverallStatus,
				deployStatus.DeploymentResults.Reason),
		)
		return
	}

	mapFeatureStatusToState(deployStatus, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read refreshes state from GET /ssp/lcm/features/{feature}/status.
func (r *FeatureResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data FeatureResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	feature := data.Feature.ValueString()
	var status client.FeatureDeploymentStatus
	httpStatus, err := r.client.Get(ctx, "/ssp/lcm/features/"+feature+"/status", &status)
	if httpStatus == 404 {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading feature status", err.Error())
		return
	}

	// If the feature is no longer deployed (drifted out of band), remove from state.
	if status.OverallStatus == "NOT_DEPLOYED" {
		resp.State.RemoveResource(ctx)
		return
	}

	mapFeatureStatusToState(&status, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update is a no-op: the only mutable attribute is `feature` which forces replacement.
func (r *FeatureResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

// Delete triggers UNDEPLOY and waits for the feature to reach NOT_DEPLOYED.
func (r *FeatureResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data FeatureResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	feature := data.Feature.ValueString()

	action := "UNDEPLOY"
	if data.ForceUndeploy.ValueBool() {
		action = "FORCE_UNDEPLOY"
	}

	lcmMu.Lock()
	defer lcmMu.Unlock()

	revision, err := r.getRevision(ctx, feature)
	if err != nil {
		resp.Diagnostics.AddError("Error reading feature revision before undeploy", err.Error())
		return
	}

	_, err = r.client.Put(ctx, "/ssp/lcm/features/"+feature, &client.FeatureDeployment{
		Revision: revision,
		Action:   action,
	}, nil)
	if err != nil {
		resp.Diagnostics.AddError("Error triggering undeploy for feature "+feature, err.Error())
		return
	}

	undeployStatus, err := r.client.WaitForFeatureDeployment(ctx, feature)
	if err != nil {
		resp.Diagnostics.AddError("Error waiting for feature undeployment", err.Error())
		return
	}

	if undeployStatus.OverallStatus == "UNDEPLOYMENT_FAILED" {
		resp.Diagnostics.AddError(
			"Undeployment failed for feature "+feature,
			fmt.Sprintf("Reason: %s", undeployStatus.DeploymentResults.Reason),
		)
	}
}

func (r *FeatureResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	feature := req.ID

	var status client.FeatureDeploymentStatus
	_, err := r.client.Get(ctx, "/ssp/lcm/features/"+feature+"/status", &status)
	if err != nil {
		resp.Diagnostics.AddError("Error reading feature status during import", err.Error())
		return
	}

	// force_undeploy has no server-side representation to read back; default
	// it to false on import, matching the schema's Default for a fresh plan.
	data := FeatureResourceModel{Feature: types.StringValue(feature), ForceUndeploy: types.BoolValue(false)}
	mapFeatureStatusToState(&status, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// getRevision fetches the current _revision for a feature.
func (r *FeatureResource) getRevision(ctx context.Context, feature string) (int, error) {
	var fd client.FeatureDeployment
	_, err := r.client.Get(ctx, "/ssp/lcm/features/"+feature, &fd)
	if err != nil {
		return 0, fmt.Errorf("GET /ssp/lcm/features/%s: %w", feature, err)
	}
	return fd.Revision, nil
}

func mapFeatureStatusToState(s *client.FeatureDeploymentStatus, m *FeatureResourceModel) {
	m.ID = m.Feature
	m.OverallStatus = types.StringValue(s.OverallStatus)
	m.OverallProgress = types.Int64Value(int64(s.OverallProgress))
	m.PrecheckStatus = types.StringValue(s.PrecheckResults.OverallStatus)
	m.DeploymentStatus = types.StringValue(s.DeploymentResults.DeploymentStatus)
}
