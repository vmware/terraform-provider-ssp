// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ resource.Resource = &RestoreResource{}

func NewRestoreResource() resource.Resource {
	return &RestoreResource{}
}

// RestoreResource manages system restore execution for the SSP platform.
type RestoreResource struct {
	client *client.Client
}

// RestoreResourceModel is the Terraform state model for system restore.
type RestoreResourceModel struct {
	ID           types.String `tfsdk:"id"`
	BackupID     types.String `tfsdk:"backup_id"`
	ForceRestore types.Bool   `tfsdk:"force_restore"`
	Action       types.String `tfsdk:"action"`
	Status       types.String `tfsdk:"status"`
	Progress     types.Int64  `tfsdk:"progress"`
}

func (r *RestoreResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_restore"
}

func (r *RestoreResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Restores the SSP platform from a previously created backup.\n\n" +
			"Corresponds to `POST /ssp/restore` and `GET /ssp/restore/status/{id}`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique ID of the restore execution job.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"backup_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Unique ID of the successful backup file to restore from.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"force_restore": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "When true, bypasses non-fatal validation warnings during restore.",
			},
			"action": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Restore action trigger. Defaults to `RESTORE`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Status of the restore execution (`SUCCESS`, `FAILED`, `IN_PROGRESS`).",
			},
			"progress": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Percentage progress of the restore operation.",
			},
		},
	}
}

func (r *RestoreResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RestoreResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RestoreResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	action := "RESTORE"
	if !data.Action.IsNull() && data.Action.ValueString() != "" {
		action = data.Action.ValueString()
	}

	reqPayload := client.RestoreRequest{
		BackupID:     data.BackupID.ValueString(),
		ForceRestore: data.ForceRestore.ValueBool(),
		Action:       action,
	}

	// Restore competes with feature/site/upgrade/backup LCM actions for the SSP
	// cluster's single-concurrent-lifecycle-action slot; serialize with them.
	lcmMu.Lock()
	defer lcmMu.Unlock()

	var asyncResp client.AsyncApiResponse
	_, err := r.client.Post(ctx, "/ssp/restore", reqPayload, &asyncResp)
	if err != nil {
		resp.Diagnostics.AddError("Error initiating restore", err.Error())
		return
	}

	restoreID := asyncResp.ID
	if restoreID == "" {
		resp.Diagnostics.AddError("No restore ID returned", "API returned an empty ID after triggering restore.")
		return
	}

	data.ID = types.StringValue(restoreID)
	data.Action = types.StringValue(action)

	status, waitErr := r.client.WaitForRestoreComplete(ctx, restoreID)
	if status != nil {
		data.Status = types.StringValue(status.Status)
		data.Progress = types.Int64Value(int64(status.Progress))
	} else {
		// The API already accepted and started this job (restoreID is real)
		// even though the poll below didn't observe a terminal status.
		data.Status = types.StringValue("IN_PROGRESS")
	}

	// Persist state now, regardless of the poll outcome: the restore job was
	// genuinely created server-side, so losing track of its ID here would
	// cause the next apply to POST a second, duplicate/orphaned restore job.
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if waitErr != nil {
		resp.Diagnostics.AddError("Restore operation failed", waitErr.Error())
	}
}

func (r *RestoreResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RestoreResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var status client.RestoreStatus
	httpStatus, err := r.client.Get(ctx, "/ssp/restore/status/"+data.ID.ValueString(), &status)
	if httpStatus == 404 {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading restore status", err.Error())
		return
	}

	data.Status = types.StringValue(status.Status)
	data.Progress = types.Int64Value(int64(status.Progress))
	if status.BackupID != "" {
		data.BackupID = types.StringValue(status.BackupID)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RestoreResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update not supported", "Restore resources are immutable and cannot be updated.")
}

func (r *RestoreResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Restore logs are audit trails and are not deleted from backend on destroy.
}
