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

var _ datasource.DataSource = &ServicesStatusDataSource{}

func NewServicesStatusDataSource() datasource.DataSource {
	return &ServicesStatusDataSource{}
}

type ServicesStatusDataSource struct {
	client *client.Client
}

type ServicesStatusDataSourceModel struct {
	Data             types.List   `tfsdk:"data"`
	MessagingService types.Object `tfsdk:"messaging_service"`
}

var serviceStatusAttrTypes = map[string]attr.Type{
	"service_name": types.StringType,
	"health":       types.StringType,
}

var messagingServiceAttrTypes = map[string]attr.Type{
	"status": types.StringType,
}

func (d *ServicesStatusDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_services_status"
}

func (d *ServicesStatusDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves the status of individual platform service categories and the messaging service.\n\n" +
			"Corresponds to `GET /ssp/cluster/monitor/services/status`.",
		Attributes: map[string]schema.Attribute{
			"data": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Status of each platform service category.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"service_name": schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the service category."},
						"health":       schema.StringAttribute{Computed: true, MarkdownDescription: "Health status of the service category."},
					},
				},
			},
			"messaging_service": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Status and metrics of the platform messaging service.",
				Attributes: map[string]schema.Attribute{
					"status": schema.StringAttribute{Computed: true, MarkdownDescription: "Health status of the messaging service."},
				},
			},
		},
	}
}

func (d *ServicesStatusDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ServicesStatusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ServicesStatusDataSourceModel

	var list client.ServiceStatusList
	_, err := d.client.Get(ctx, "/ssp/cluster/monitor/services/status", &list)
	if err != nil {
		resp.Diagnostics.AddError("Error reading services status", err.Error())
		return
	}

	serviceObjs := make([]attr.Value, len(list.Data))
	for i, s := range list.Data {
		obj, diags := types.ObjectValue(serviceStatusAttrTypes, map[string]attr.Value{
			"service_name": types.StringValue(s.ServiceName),
			"health":       types.StringValue(s.Health),
		})
		resp.Diagnostics.Append(diags...)
		serviceObjs[i] = obj
	}
	dataList, listDiags := types.ListValue(types.ObjectType{AttrTypes: serviceStatusAttrTypes}, serviceObjs)
	resp.Diagnostics.Append(listDiags...)
	data.Data = dataList

	messagingStatus := ""
	if list.MessagingService != nil {
		messagingStatus = list.MessagingService.Status
	}
	msgObj, msgDiags := types.ObjectValue(messagingServiceAttrTypes, map[string]attr.Value{
		"status": types.StringValue(messagingStatus),
	})
	resp.Diagnostics.Append(msgDiags...)
	data.MessagingService = msgObj

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
