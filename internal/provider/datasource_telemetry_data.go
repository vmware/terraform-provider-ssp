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

var _ datasource.DataSource = &TelemetryDataDataSource{}

func NewTelemetryDataDataSource() datasource.DataSource {
	return &TelemetryDataDataSource{}
}

type TelemetryDataDataSource struct {
	client *client.Client
}

type TelemetryDataDataSourceModel struct {
	Format types.String `tfsdk:"format"`
	Data   types.String `tfsdk:"data"`
}

func (d *TelemetryDataDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_telemetry_data"
}

func (d *TelemetryDataDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Exports the platform's aggregated telemetry data as a raw file dump — not a " +
			"structured resource. `data` is the exact bytes returned by the API, decoded as a string " +
			"(a CSV document by default); this data source does not parse or interpret its contents.\n\n" +
			"Corresponds to `GET /ssp/telemetry/data`.",
		Attributes: map[string]schema.Attribute{
			"format": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Export format to request from the API. Defaults to `csv`.",
			},
			"data": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Raw exported telemetry data, in the requested `format`.",
			},
		},
	}
}

func (d *TelemetryDataDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *TelemetryDataDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data TelemetryDataDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	format := "csv"
	if !data.Format.IsNull() && data.Format.ValueString() != "" {
		format = data.Format.ValueString()
	}

	body, _, err := d.client.GetRaw(ctx, "/ssp/telemetry/data?format="+format)
	if err != nil {
		resp.Diagnostics.AddError("Error reading telemetry data", err.Error())
		return
	}

	data.Format = types.StringValue(format)
	data.Data = types.StringValue(string(body))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
