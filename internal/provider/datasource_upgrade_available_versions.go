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

var _ datasource.DataSource = &UpgradeAvailableVersionsDataSource{}

func NewUpgradeAvailableVersionsDataSource() datasource.DataSource {
	return &UpgradeAvailableVersionsDataSource{}
}

type UpgradeAvailableVersionsDataSource struct {
	client *client.Client
}

type UpgradeAvailableVersionsDataSourceModel struct {
	Versions []types.String `tfsdk:"versions"`
}

func (d *UpgradeAvailableVersionsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_upgrade_available_versions"
}

func (d *UpgradeAvailableVersionsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists available target software versions for upgrading the SSP platform.\n\n" +
			"Corresponds to `GET /ssp/upgrade/available-versions`.",
		Attributes: map[string]schema.Attribute{
			"versions": schema.ListAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "List of version strings available for upgrade.",
			},
		},
	}
}

func (d *UpgradeAvailableVersionsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *UpgradeAvailableVersionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data UpgradeAvailableVersionsDataSourceModel

	var list client.AvailableVersionList
	_, err := d.client.Get(ctx, "/ssp/upgrade/available-versions", &list)
	if err != nil {
		resp.Diagnostics.AddError("Error fetching available upgrade versions", err.Error())
		return
	}

	data.Versions = make([]types.String, len(list.Versions))
	for i, v := range list.Versions {
		data.Versions[i] = types.StringValue(v)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
