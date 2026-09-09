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

var _ datasource.DataSource = &AlarmsDataSource{}

func NewAlarmsDataSource() datasource.DataSource {
	return &AlarmsDataSource{}
}

type AlarmsDataSource struct {
	client *client.Client
}

type AlarmsDataSourceModel struct {
	FeatureNames types.List `tfsdk:"feature_names"`
	Severities   types.List `tfsdk:"severities"`
	States       types.List `tfsdk:"states"`
	Results      types.List `tfsdk:"results"`
}

var alarmInstanceAttrTypes = map[string]attr.Type{
	"id":            types.StringType,
	"definition_id": types.StringType,
	"feature_name":  types.StringType,
	"severity":      types.StringType,
	"summary":       types.StringType,
	"state":         types.StringType,
	"resource_id":   types.StringType,
	"value":         types.StringType,
}

func (d *AlarmsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alarms"
}

func (d *AlarmsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists alarm instances matching an optional filter.\n\n" +
			"Corresponds to `POST /ssp/alarms`.",
		Attributes: map[string]schema.Attribute{
			"feature_names": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Filter to alarm instances for these feature names.",
			},
			"severities": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Filter to alarm instances with these severities.",
			},
			"states": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Filter to alarm instances in these states (`OPEN`, `ACKNOWLEDGED`, `SUPPRESSED`, `RESOLVED`).",
			},
			"results": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Alarm instances matching the filter.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":            schema.StringAttribute{Computed: true},
						"definition_id": schema.StringAttribute{Computed: true},
						"feature_name":  schema.StringAttribute{Computed: true},
						"severity":      schema.StringAttribute{Computed: true},
						"summary":       schema.StringAttribute{Computed: true},
						"state":         schema.StringAttribute{Computed: true},
						"resource_id":   schema.StringAttribute{Computed: true},
						"value":         schema.StringAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *AlarmsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *AlarmsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data AlarmsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filter := &client.AlarmInstanceFilterRequest{}
	if !data.FeatureNames.IsNull() {
		resp.Diagnostics.Append(data.FeatureNames.ElementsAs(ctx, &filter.FeatureNames, false)...)
	}
	if !data.Severities.IsNull() {
		resp.Diagnostics.Append(data.Severities.ElementsAs(ctx, &filter.Severities, false)...)
	}
	if !data.States.IsNull() {
		resp.Diagnostics.Append(data.States.ElementsAs(ctx, &filter.States, false)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	var result client.AlarmInstanceListResult
	_, err := d.client.Post(ctx, "/ssp/alarms", filter, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error listing alarm instances", err.Error())
		return
	}

	objs := make([]attr.Value, len(result.Results))
	for i, inst := range result.Results {
		obj, diags := types.ObjectValue(alarmInstanceAttrTypes, map[string]attr.Value{
			"id":            types.StringValue(inst.ID),
			"definition_id": types.StringValue(inst.DefinitionID),
			"feature_name":  types.StringValue(inst.FeatureName),
			"severity":      types.StringValue(inst.Severity),
			"summary":       types.StringValue(inst.Summary),
			"state":         types.StringValue(inst.State),
			"resource_id":   types.StringValue(inst.ResourceID),
			"value":         types.StringValue(inst.Value),
		})
		resp.Diagnostics.Append(diags...)
		objs[i] = obj
	}
	resultsList, diags := types.ListValue(types.ObjectType{AttrTypes: alarmInstanceAttrTypes}, objs)
	resp.Diagnostics.Append(diags...)
	if !resp.Diagnostics.HasError() {
		data.Results = resultsList
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
