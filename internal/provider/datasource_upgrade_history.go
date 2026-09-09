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

var _ datasource.DataSource = &UpgradeHistoryDataSource{}

func NewUpgradeHistoryDataSource() datasource.DataSource {
	return &UpgradeHistoryDataSource{}
}

type UpgradeHistoryDataSource struct {
	client *client.Client
}

type UpgradeHistoryDataSourceModel struct {
	History []UpgradeHistoryItemModel `tfsdk:"history"`
}

type UpgradeHistoryItemModel struct {
	ID            types.String `tfsdk:"id"`
	TargetVersion types.String `tfsdk:"target_version"`
	Status        types.String `tfsdk:"status"`
	CurrentStep   types.String `tfsdk:"current_step"`
}

func (d *UpgradeHistoryDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_upgrade_history"
}

func (d *UpgradeHistoryDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists past upgrade execution history for the SSP platform.\n\n" +
			"Corresponds to `GET /ssp/upgrade/history`.",
		Attributes: map[string]schema.Attribute{
			"history": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of past upgrade execution records.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":             schema.StringAttribute{Computed: true, MarkdownDescription: "Upgrade record ID."},
						"target_version": schema.StringAttribute{Computed: true, MarkdownDescription: "Target version of the upgrade."},
						"status":         schema.StringAttribute{Computed: true, MarkdownDescription: "Final or current status."},
						"current_step":   schema.StringAttribute{Computed: true, MarkdownDescription: "Current step at completion."},
					},
				},
			},
		},
	}
}

func (d *UpgradeHistoryDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *UpgradeHistoryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data UpgradeHistoryDataSourceModel

	var list client.UpgradeHistoryList
	_, err := d.client.Get(ctx, "/ssp/upgrade/history", &list)
	if err != nil {
		resp.Diagnostics.AddError("Error fetching upgrade history", err.Error())
		return
	}

	data.History = make([]UpgradeHistoryItemModel, len(list.Results))
	for i, h := range list.Results {
		data.History[i] = UpgradeHistoryItemModel{
			ID:            types.StringValue(h.ID),
			TargetVersion: types.StringValue(h.TargetVersion),
			Status:        types.StringValue(h.Status),
			CurrentStep:   types.StringValue(h.CurrentStep),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
