// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ datasource.DataSource = &RestoreStatusDataSource{}

func NewRestoreStatusDataSource() datasource.DataSource {
	return &RestoreStatusDataSource{}
}

type RestoreStatusDataSource struct {
	client *client.Client
}

type RestoreStatusDataSourceModel struct {
	Results []RestoreStatusModel `tfsdk:"results"`
}

type RestoreStatusModel struct {
	ID              types.String `tfsdk:"id"`
	BackupID        types.String `tfsdk:"backup_id"`
	Status          types.String `tfsdk:"status"`
	Progress        types.Int64  `tfsdk:"progress"`
	ProgressMessage types.String `tfsdk:"progress_message"`
	ErrorMessages   types.List   `tfsdk:"error_messages"`
	NoOfEntities    types.Int64  `tfsdk:"no_of_entities"`
}

func (d *RestoreStatusDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_restore_status"
}

func (d *RestoreStatusDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists restore history and the status of recent restore jobs for the SSP platform.\n\n" +
			"Corresponds to `GET /ssp/restore/status`.",
		Attributes: map[string]schema.Attribute{
			"results": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of restore job records, newest first.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":               schema.StringAttribute{Computed: true, MarkdownDescription: "Restore job UUID."},
						"backup_id":        schema.StringAttribute{Computed: true, MarkdownDescription: "UUID of the backup this restore was sourced from."},
						"status":           schema.StringAttribute{Computed: true, MarkdownDescription: "Job status: IN_PROGRESS, SUCCESS, FAILED."},
						"progress":         schema.Int64Attribute{Computed: true, MarkdownDescription: "Completion percentage (0–100)."},
						"progress_message": schema.StringAttribute{Computed: true, MarkdownDescription: "Human-readable progress description."},
						"error_messages": schema.ListAttribute{
							Computed:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Error messages encountered during the restore operation, if any.",
						},
						"no_of_entities": schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of managed entities restored."},
					},
				},
			},
		},
	}
}

func (d *RestoreStatusDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *RestoreStatusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RestoreStatusDataSourceModel

	var list client.RestoreStatusList
	_, err := d.client.Get(ctx, "/ssp/restore/status", &list)
	if err != nil {
		resp.Diagnostics.AddError("Error reading restore status", err.Error())
		return
	}

	data.Results = make([]RestoreStatusModel, len(list.Results))
	for i, rs := range list.Results {
		errMsgs, diags := types.ListValueFrom(ctx, types.StringType, rs.ErrorMessages)
		resp.Diagnostics.Append(diags...)
		data.Results[i] = RestoreStatusModel{
			ID:              types.StringValue(rs.ID),
			BackupID:        types.StringValue(rs.BackupID),
			Status:          types.StringValue(rs.Status),
			Progress:        types.Int64Value(int64(rs.Progress)),
			ProgressMessage: types.StringValue(rs.ProgressMessage),
			ErrorMessages:   errMsgs,
			NoOfEntities:    types.Int64Value(int64(rs.NoOfEntities)),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
