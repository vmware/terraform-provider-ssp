package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ datasource.DataSource = &FeatureHealthDataSource{}

func NewFeatureHealthDataSource() datasource.DataSource {
	return &FeatureHealthDataSource{}
}

type FeatureHealthDataSource struct {
	client *client.Client
}

type FeatureHealthDataSourceModel struct {
	OverallStatus types.String `tfsdk:"overall_status"`
	Features      types.Set    `tfsdk:"features"`
}

var featureHealthAttrTypes = map[string]attr.Type{
	"name":   types.StringType,
	"status": types.StringType,
}

func (d *FeatureHealthDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_feature_health"
}

func (d *FeatureHealthDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves hierarchical feature and component health status for the SSP platform.\n\n" +
			"Corresponds to `GET /ssp/cluster/monitor/feature/health`.",
		Attributes: map[string]schema.Attribute{
			"overall_status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Overall platform health aggregated from all features: `UP`, `PARTIALLY_UP`, or `DOWN`.",
			},
			"features": schema.SetNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Set of platform features with their individual health status.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":   schema.StringAttribute{Computed: true, MarkdownDescription: "Feature name."},
						"status": schema.StringAttribute{Computed: true, MarkdownDescription: "Feature health status."},
					},
				},
			},
		},
	}
}

func (d *FeatureHealthDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *FeatureHealthDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data FeatureHealthDataSourceModel

	var health client.FeatureHealthResponse
	_, err := d.client.Get(ctx, "/ssp/cluster/monitor/feature/health", &health)
	if err != nil {
		resp.Diagnostics.AddError("Error reading feature health", err.Error())
		return
	}

	data.OverallStatus = types.StringValue(health.OverallStatus)

	featureObjs := make([]attr.Value, len(health.Features))
	for i, f := range health.Features {
		obj, diags := types.ObjectValue(featureHealthAttrTypes, map[string]attr.Value{
			"name":   types.StringValue(f.Name),
			"status": types.StringValue(f.Status),
		})
		resp.Diagnostics.Append(diags...)
		featureObjs[i] = obj
	}
	featureSet, setDiags := types.SetValue(types.ObjectType{AttrTypes: featureHealthAttrTypes}, featureObjs)
	resp.Diagnostics.Append(setDiags...)
	if !resp.Diagnostics.HasError() {
		data.Features = featureSet
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
