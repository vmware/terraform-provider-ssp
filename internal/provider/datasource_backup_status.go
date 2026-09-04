package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ datasource.DataSource = &BackupStatusDataSource{}

func NewBackupStatusDataSource() datasource.DataSource {
	return &BackupStatusDataSource{}
}

type BackupStatusDataSource struct {
	client *client.Client
}

type BackupStatusDataSourceModel struct {
	Results []BackupStatusModel `tfsdk:"results"`
}

type BackupStatusModel struct {
	ID                string       `tfsdk:"-"`
	BackupID          types.String `tfsdk:"backup_id"`
	Status            types.String `tfsdk:"status"`
	BackupType        types.String `tfsdk:"backup_type"`
	BackupTriggerType types.String `tfsdk:"backup_trigger_type"`
	BackupStartTime   types.Int64  `tfsdk:"backup_start_time"`
	BackupEndTime     types.Int64  `tfsdk:"backup_end_time"`
	Progress          types.Int64  `tfsdk:"progress"`
	ProgressMessage   types.String `tfsdk:"progress_message"`
	Version           types.String `tfsdk:"version"`
}

func (d *BackupStatusDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_backup_status"
}

func (d *BackupStatusDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists backup history and the status of recent backup jobs for the SSP platform.\n\n" +
			"Corresponds to `GET /ssp/backup/status`.",
		Attributes: map[string]schema.Attribute{
			"results": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of backup job records, newest first.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"backup_id":           schema.StringAttribute{Computed: true, MarkdownDescription: "Backup job UUID."},
						"status":              schema.StringAttribute{Computed: true, MarkdownDescription: "Job status: IN_PROGRESS, SUCCESS, FAILED."},
						"backup_type":         schema.StringAttribute{Computed: true, MarkdownDescription: "Backup type: FULL_BACKUP."},
						"backup_trigger_type": schema.StringAttribute{Computed: true, MarkdownDescription: "Trigger: ON_DEMAND or RECURRING."},
						"backup_start_time":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Start time as Unix epoch in milliseconds."},
						"backup_end_time":     schema.Int64Attribute{Computed: true, MarkdownDescription: "End time as Unix epoch in milliseconds."},
						"progress":            schema.Int64Attribute{Computed: true, MarkdownDescription: "Completion percentage (0–100)."},
						"progress_message":    schema.StringAttribute{Computed: true, MarkdownDescription: "Human-readable progress description."},
						"version":             schema.StringAttribute{Computed: true, MarkdownDescription: "SSP version at the time of the backup."},
					},
				},
			},
		},
	}
}

func (d *BackupStatusDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, err := GetClientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected provider data type", err.Error())
		return
	}
	d.client = c
}

func (d *BackupStatusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data BackupStatusDataSourceModel

	var list client.BackupStatusList
	_, err := d.client.Get(ctx, "/ssp/backup/status", &list)
	if err != nil {
		resp.Diagnostics.AddError("Error reading backup status", err.Error())
		return
	}

	data.Results = make([]BackupStatusModel, len(list.Results))
	for i, b := range list.Results {
		data.Results[i] = BackupStatusModel{
			BackupID:          types.StringValue(b.ID),
			Status:            types.StringValue(b.Status),
			BackupType:        types.StringValue(b.BackupType),
			BackupTriggerType: types.StringValue(b.BackupTriggerType),
			BackupStartTime:   types.Int64Value(b.BackupStartTime),
			BackupEndTime:     types.Int64Value(b.BackupEndTime),
			Progress:          types.Int64Value(int64(b.Progress)),
			ProgressMessage:   types.StringValue(b.ProgressMessage),
			Version:           types.StringValue(b.Version),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
