// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ datasource.DataSource = &SiteDataSource{}

func NewSiteDataSource() datasource.DataSource {
	return &SiteDataSource{}
}

type SiteDataSource struct {
	client *client.Client
}

type SiteDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	SiteType         types.String `tfsdk:"site_type"`
	SiteName         types.String `tfsdk:"site_name"`
	CurrentState     types.String `tfsdk:"current_state"`
	ConnectionStatus types.String `tfsdk:"connection_status"`
	ClusterStatus    types.String `tfsdk:"cluster_status"`
	NsxVersion       types.String `tfsdk:"nsx_version"`
	NsxClusterID     types.String `tfsdk:"nsx_cluster_id"`
}

func (d *SiteDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_site"
}

func (d *SiteDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a specific site from the SSP platform by its ID.\n\n" +
			"Corresponds to `GET /ssp/sites/{site-id}`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of the site to retrieve.",
			},
			"site_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Site type: `NSX_MANAGER`, `AVI`, or `SSP`.",
			},
			"site_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Display name of the site.",
			},
			"current_state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current readiness: `READY`, `NOT_READY`, `INACTIVE`, or `UNKNOWN`.",
			},
			"connection_status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Connection health: `HEALTHY`, `UNHEALTHY`, or `UNKNOWN`.",
			},
			"cluster_status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Site cluster status: `STABLE`, `DEGRADED`, or `UNAVAILABLE`.",
			},
			"nsx_version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "NSX software version (NSX_MANAGER sites only).",
			},
			"nsx_cluster_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "NSX cluster ID (NSX_MANAGER sites only).",
			},
		},
	}
}

func (d *SiteDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SiteDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data SiteDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var site client.Site
	status, err := d.client.Get(ctx, "/ssp/sites/"+data.ID.ValueString(), &site)
	if status == 404 {
		resp.Diagnostics.AddError("Site not found", fmt.Sprintf("No site with ID %s", data.ID.ValueString()))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading site", err.Error())
		return
	}

	data.SiteType = types.StringValue(site.SiteType)
	data.SiteName = types.StringValue(site.SiteName)
	data.CurrentState = types.StringValue(site.CurrentState)

	if site.Status != nil {
		data.ConnectionStatus = types.StringValue(site.Status.ConnectionStatus)
		data.ClusterStatus = types.StringValue(site.Status.ClusterStatus)
		data.NsxVersion = types.StringValue(site.Status.NsxVersion)
		data.NsxClusterID = types.StringValue(site.Status.NsxClusterID)
	} else {
		data.ConnectionStatus = types.StringValue("")
		data.ClusterStatus = types.StringValue("")
		data.NsxVersion = types.StringValue("")
		data.NsxClusterID = types.StringValue("")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
