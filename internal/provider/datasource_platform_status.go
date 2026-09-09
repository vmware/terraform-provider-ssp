// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var networkDataFlowAttrTypes = map[string]attr.Type{
	"transmit": types.Float64Type,
	"receive":  types.Float64Type,
	"total":    types.Float64Type,
}

var _ datasource.DataSource = &PlatformStatusDataSource{}

func NewPlatformStatusDataSource() datasource.DataSource {
	return &PlatformStatusDataSource{}
}

type PlatformStatusDataSource struct {
	client *client.Client
}

type PlatformStatusDataSourceModel struct {
	ClusterID          types.String `tfsdk:"cluster_id"`
	ClusterName        types.String `tfsdk:"cluster_name"`
	ProductVersion     types.String `tfsdk:"product_version"`
	NodeCount          types.Int64  `tfsdk:"node_count"`
	FormFactor         types.String `tfsdk:"form_factor"`
	Health             types.String `tfsdk:"health"`
	MessageBusEndpoint types.String `tfsdk:"message_bus_endpoint"`
	K8sVersion         types.String `tfsdk:"k8s_version"`
	IngressURL         types.String `tfsdk:"ingress_url"`
	NetworkDataFlow    types.Object `tfsdk:"network_data_flow"`
}

func (d *PlatformStatusDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_platform_status"
}

func (d *PlatformStatusDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves comprehensive SSP cluster status: health, config, form factor, and version.\n\n" +
			"Corresponds to `GET /ssp/cluster/monitor/platform/status`.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique cluster identifier.",
			},
			"cluster_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Display name of the SSP cluster.",
			},
			"product_version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "SSP product version string.",
			},
			"node_count": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Total number of nodes in the cluster.",
			},
			"form_factor": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current form factor (e.g. MEDIUM, LARGE, EXTRA_LARGE).",
			},
			"health": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Overall cluster health: `UP`, `PARTIALLY_UP`, or `DOWN`.",
			},
			"message_bus_endpoint": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Endpoint of the platform message bus (FQDN:Port or IP:Port).",
			},
			"k8s_version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Version of the underlying Kubernetes infrastructure the platform is built on.",
			},
			"ingress_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Endpoint for cluster ingress.",
			},
			"network_data_flow": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Network data flow statistics for the cluster.",
				Attributes: map[string]schema.Attribute{
					"transmit": schema.Float64Attribute{Computed: true, MarkdownDescription: "Network data transmit rate."},
					"receive":  schema.Float64Attribute{Computed: true, MarkdownDescription: "Network data receive rate."},
					"total":    schema.Float64Attribute{Computed: true, MarkdownDescription: "Total of transmit and receive data rates."},
				},
			},
		},
	}
}

func (d *PlatformStatusDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *PlatformStatusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data PlatformStatusDataSourceModel

	var status client.ClusterStatus
	_, err := d.client.Get(ctx, "/ssp/cluster/monitor/platform/status", &status)
	if err != nil {
		resp.Diagnostics.AddError("Error reading platform status", err.Error())
		return
	}

	data.ClusterID = types.StringValue(status.ClusterID)
	data.ClusterName = types.StringValue(status.ClusterName)
	data.ProductVersion = types.StringValue(status.ProductVersion)
	data.NodeCount = types.Int64Value(int64(status.NodeCount))
	data.FormFactor = types.StringValue(status.FormFactor)
	data.Health = types.StringValue(status.Health)
	data.MessageBusEndpoint = types.StringValue(status.MessageBusEndpoint)
	data.K8sVersion = types.StringValue(status.K8sVersion)
	data.IngressURL = types.StringValue(status.IngressURL)

	if status.NetworkDataFlow != nil {
		networkObj, d := types.ObjectValue(networkDataFlowAttrTypes, map[string]attr.Value{
			"transmit": types.Float64Value(status.NetworkDataFlow.Transmit),
			"receive":  types.Float64Value(status.NetworkDataFlow.Receive),
			"total":    types.Float64Value(status.NetworkDataFlow.Total),
		})
		resp.Diagnostics.Append(d...)
		data.NetworkDataFlow = networkObj
	} else {
		data.NetworkDataFlow = types.ObjectNull(networkDataFlowAttrTypes)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
