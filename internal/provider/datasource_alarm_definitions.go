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

var _ datasource.DataSource = &AlarmDefinitionsDataSource{}

func NewAlarmDefinitionsDataSource() datasource.DataSource {
	return &AlarmDefinitionsDataSource{}
}

type AlarmDefinitionsDataSource struct {
	client *client.Client
}

type AlarmDefinitionsDataSourceModel struct {
	FeatureNames types.List `tfsdk:"feature_names"`
	Severities   types.List `tfsdk:"severities"`
	EventTypes   types.List `tfsdk:"event_types"`
	Enabled      types.Bool `tfsdk:"enabled"`
	Results      types.List `tfsdk:"results"`
}

var alarmDefinitionAttrTypes = map[string]attr.Type{
	"id":                   types.StringType,
	"feature_name":         types.StringType,
	"feature_display_name": types.StringType,
	"event_type":           types.StringType,
	"severity":             types.StringType,
	"summary":              types.StringType,
	"enabled":              types.BoolType,
}

func (d *AlarmDefinitionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alarm_definitions"
}

func (d *AlarmDefinitionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists alarm definitions matching an optional filter.\n\n" +
			"Corresponds to `POST /ssp/alarms/definitions`.",
		Attributes: map[string]schema.Attribute{
			"feature_names": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Filter to alarm definitions for these feature names.",
			},
			"severities": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Filter to alarm definitions with these severities.",
			},
			"event_types": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Filter to alarm definitions with these event types.",
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Filter to alarm definitions with this enabled state.",
			},
			"results": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Alarm definitions matching the filter.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                   schema.StringAttribute{Computed: true},
						"feature_name":         schema.StringAttribute{Computed: true},
						"feature_display_name": schema.StringAttribute{Computed: true},
						"event_type":           schema.StringAttribute{Computed: true},
						"severity":             schema.StringAttribute{Computed: true},
						"summary":              schema.StringAttribute{Computed: true},
						"enabled":              schema.BoolAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *AlarmDefinitionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *AlarmDefinitionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data AlarmDefinitionsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filter := &client.AlarmDefinitionFilterRequest{}
	if !data.FeatureNames.IsNull() {
		resp.Diagnostics.Append(data.FeatureNames.ElementsAs(ctx, &filter.FeatureNames, false)...)
	}
	if !data.Severities.IsNull() {
		resp.Diagnostics.Append(data.Severities.ElementsAs(ctx, &filter.Severities, false)...)
	}
	if !data.EventTypes.IsNull() {
		resp.Diagnostics.Append(data.EventTypes.ElementsAs(ctx, &filter.EventTypes, false)...)
	}
	if !data.Enabled.IsNull() {
		v := data.Enabled.ValueBool()
		filter.Enabled = &v
	}
	if resp.Diagnostics.HasError() {
		return
	}

	var result client.AlarmDefinitionListResult
	_, err := d.client.Post(ctx, "/ssp/alarms/definitions", filter, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error listing alarm definitions", err.Error())
		return
	}

	objs := make([]attr.Value, len(result.Results))
	for i, def := range result.Results {
		obj, diags := types.ObjectValue(alarmDefinitionAttrTypes, map[string]attr.Value{
			"id":                   types.StringValue(def.ID),
			"feature_name":         types.StringValue(def.FeatureName),
			"feature_display_name": types.StringValue(def.FeatureDisplayName),
			"event_type":           types.StringValue(def.EventType),
			"severity":             types.StringValue(def.Severity),
			"summary":              types.StringValue(def.Summary),
			"enabled":              types.BoolValue(def.Enabled),
		})
		resp.Diagnostics.Append(diags...)
		objs[i] = obj
	}
	resultsList, diags := types.ListValue(types.ObjectType{AttrTypes: alarmDefinitionAttrTypes}, objs)
	resp.Diagnostics.Append(diags...)
	if !resp.Diagnostics.HasError() {
		data.Results = resultsList
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
