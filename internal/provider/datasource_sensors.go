package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ datasource.DataSource = &SensorsDataSource{}

func NewSensorsDataSource() datasource.DataSource {
	return &SensorsDataSource{}
}

type SensorsDataSource struct {
	client *client.Client
}

type SensorsDataSourceModel struct {
	Results []SensorModel `tfsdk:"results"`
}

type SensorModel struct {
	ID          types.String `tfsdk:"id"`
	DisplayName types.String `tfsdk:"display_name"`
	Description types.String `tfsdk:"description"`
	TokenID     types.String `tfsdk:"token_id"`
	Algorithm   types.String `tfsdk:"algorithm"`
	KeySize     types.Int64  `tfsdk:"key_size"`
}

func (d *SensorsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sensors"
}

func (d *SensorsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all sensor appliances registered with the SSP platform.\n\n" +
			"Corresponds to `GET /sensors/appliances`.",
		Attributes: map[string]schema.Attribute{
			"results": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of all registered sensor appliances.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":           schema.StringAttribute{Computed: true, MarkdownDescription: "Sensor UUID."},
						"display_name": schema.StringAttribute{Computed: true, MarkdownDescription: "Sensor display name."},
						"description":  schema.StringAttribute{Computed: true, MarkdownDescription: "Sensor description."},
						"token_id":     schema.StringAttribute{Computed: true, MarkdownDescription: "UUID of the registration token used to register this sensor."},
						"algorithm":    schema.StringAttribute{Computed: true, MarkdownDescription: "Cryptographic algorithm used by the sensor certificate (e.g. RSA)."},
						"key_size":     schema.Int64Attribute{Computed: true, MarkdownDescription: "Key size in bits of the sensor certificate."},
					},
				},
			},
		},
	}
}

func (d *SensorsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SensorsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data SensorsDataSourceModel

	var list client.SensorList
	_, err := d.client.Get(ctx, "/sensors/appliances", &list)
	if err != nil {
		resp.Diagnostics.AddError("Error listing sensors", err.Error())
		return
	}

	data.Results = make([]SensorModel, len(list.Results))
	for i, s := range list.Results {
		data.Results[i] = SensorModel{
			ID:          types.StringValue(s.ID),
			DisplayName: types.StringValue(s.DisplayName),
			Description: types.StringValue(s.Description),
			TokenID:     types.StringValue(s.TokenID),
			Algorithm:   types.StringValue(s.Algorithm),
			KeySize:     types.Int64Value(int64(s.KeySize)),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
