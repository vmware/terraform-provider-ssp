package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ resource.Resource = &BackupResource{}

func NewBackupResource() resource.Resource {
	return &BackupResource{}
}

// BackupResource manages on-demand backup execution for the SSP platform.
type BackupResource struct {
	client *client.Client
}

// BackupResourceModel is the Terraform state model for on-demand backup.
type BackupResourceModel struct {
	ID          types.String `tfsdk:"id"`
	BackupType  types.String `tfsdk:"backup_type"`
	Action      types.String `tfsdk:"action"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Status      types.String `tfsdk:"status"`
	Progress    types.Int64  `tfsdk:"progress"`
}

func (r *BackupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_backup"
}

func (r *BackupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Triggers an on-demand backup for the SSP platform.\n\n" +
			"Corresponds to `POST /ssp/backup` and `GET /ssp/backup/status/{id}`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique ID of the backup execution job.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"backup_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Type of backup to take. The SSP API currently supports only `FULL_BACKUP`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators: []validator.String{
					stringvalidator.OneOf("FULL_BACKUP"),
				},
			},
			"action": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Backup action type. The SSP API currently supports only `BACKUP`. Defaults to `BACKUP`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators: []validator.String{
					stringvalidator.OneOf("BACKUP"),
				},
			},
			"name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional descriptive name for the backup.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional description for the backup.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Status of the backup execution (`SUCCESS`, `FAILED`, `IN_PROGRESS`).",
			},
			"progress": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Percentage progress of the backup operation.",
			},
		},
	}
}

func (r *BackupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *BackupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data BackupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	action := "BACKUP"
	if !data.Action.IsNull() && data.Action.ValueString() != "" {
		action = data.Action.ValueString()
	}

	reqPayload := client.BackupRequest{
		BackupType:  data.BackupType.ValueString(),
		Action:      action,
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
	}

	// Backup competes with feature/site/upgrade LCM actions for the SSP
	// cluster's single-concurrent-lifecycle-action slot; serialize with them.
	lcmMu.Lock()
	defer lcmMu.Unlock()

	var asyncResp client.AsyncApiResponse
	_, err := r.client.Post(ctx, "/ssp/backup", reqPayload, &asyncResp)
	if err != nil {
		resp.Diagnostics.AddError("Error initiating backup", err.Error())
		return
	}

	backupID := asyncResp.ID
	if backupID == "" {
		resp.Diagnostics.AddError("No backup ID returned", "API returned an empty ID after triggering backup.")
		return
	}

	data.ID = types.StringValue(backupID)
	data.Action = types.StringValue(action)

	status, waitErr := r.client.WaitForBackupComplete(ctx, backupID)
	if status != nil {
		data.Status = types.StringValue(status.Status)
		data.Progress = types.Int64Value(int64(status.Progress))
	} else {
		// The API already accepted and started this job (backupID is real)
		// even though the poll below didn't observe a terminal status.
		data.Status = types.StringValue("IN_PROGRESS")
	}

	// Persist state now, regardless of the poll outcome: the backup job was
	// genuinely created server-side, so losing track of its ID here would
	// cause the next apply to POST a second, duplicate/orphaned backup job.
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if waitErr != nil {
		resp.Diagnostics.AddError("Backup operation failed", waitErr.Error())
	}
}

func (r *BackupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data BackupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var status client.BackupStatus
	httpStatus, err := r.client.Get(ctx, "/ssp/backup/status/"+data.ID.ValueString(), &status)
	if httpStatus == 404 {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading backup status", err.Error())
		return
	}

	data.Status = types.StringValue(status.Status)
	data.Progress = types.Int64Value(int64(status.Progress))
	if status.BackupType != "" {
		data.BackupType = types.StringValue(status.BackupType)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BackupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update not supported", "Backup resources are immutable and cannot be updated.")
}

func (r *BackupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Backup records are audit trails and are not deleted from backend on destroy.
}
