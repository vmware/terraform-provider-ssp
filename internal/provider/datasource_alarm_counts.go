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

var _ datasource.DataSource = &AlarmCountsDataSource{}

func NewAlarmCountsDataSource() datasource.DataSource {
	return &AlarmCountsDataSource{}
}

type AlarmCountsDataSource struct {
	client *client.Client
}

type AlarmCountsDataSourceModel struct {
	FeatureNames               types.List  `tfsdk:"feature_names"`
	OpenInstanceCounts         types.List  `tfsdk:"open_instance_counts"`
	SuppressedInstanceCounts   types.List  `tfsdk:"suppressed_instance_counts"`
	ResolvedInstanceCounts     types.List  `tfsdk:"resolved_instance_counts"`
	AcknowledgedInstanceCounts types.List  `tfsdk:"acknowledged_instance_counts"`
	TotalInstanceCount         types.Int64 `tfsdk:"total_instance_count"`
}

var severityCountAttrTypes = map[string]attr.Type{
	"severity": types.StringType,
	"count":    types.Int64Type,
}

var categoryCountAttrTypes = map[string]attr.Type{
	"category": types.StringType,
	"counts":   types.ListType{ElemType: types.ObjectType{AttrTypes: severityCountAttrTypes}},
}

func (d *AlarmCountsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alarm_counts"
}

func (d *AlarmCountsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	categoryCountsAttr := schema.ListNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Instance counts grouped by category and severity.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"category": schema.StringAttribute{Computed: true, MarkdownDescription: "Category name (typically a feature name)."},
				"counts": schema.ListNestedAttribute{
					Computed:            true,
					MarkdownDescription: "Per-severity counts within this category.",
					NestedObject: schema.NestedAttributeObject{
						Attributes: map[string]schema.Attribute{
							"severity": schema.StringAttribute{Computed: true},
							"count":    schema.Int64Attribute{Computed: true},
						},
					},
				},
			},
		},
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves alarm instance counts by lifecycle state, grouped by category and severity.\n\n" +
			"Corresponds to `POST /ssp/alarms/counts`.",
		Attributes: map[string]schema.Attribute{
			"feature_names": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Filter counts to these feature names.",
			},
			"open_instance_counts":         categoryCountsAttr,
			"suppressed_instance_counts":   categoryCountsAttr,
			"resolved_instance_counts":     categoryCountsAttr,
			"acknowledged_instance_counts": categoryCountsAttr,
			"total_instance_count": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Total count of alarm instances found across all pages.",
			},
		},
	}
}

func (d *AlarmCountsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *AlarmCountsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data AlarmCountsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	filter := &client.AlarmInstanceCountFilterRequest{}
	if !data.FeatureNames.IsNull() {
		resp.Diagnostics.Append(data.FeatureNames.ElementsAs(ctx, &filter.FeatureNames, false)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	var result client.AlarmCounts
	_, err := d.client.Post(ctx, "/ssp/alarms/counts", filter, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error reading alarm counts", err.Error())
		return
	}

	setGrouped := func(grouped []client.AlarmGroupedCounts) types.List {
		var categoryObjs []attr.Value
		for _, g := range grouped {
			for _, cat := range g.Categories {
				var severityObjs []attr.Value
				if cat.SeverityCounts != nil {
					for _, sc := range cat.SeverityCounts.SeverityCounts {
						obj, diags := types.ObjectValue(severityCountAttrTypes, map[string]attr.Value{
							"severity": types.StringValue(sc.Severity),
							"count":    types.Int64Value(sc.Count),
						})
						resp.Diagnostics.Append(diags...)
						severityObjs = append(severityObjs, obj)
					}
				}
				severityList, diags := types.ListValue(types.ObjectType{AttrTypes: severityCountAttrTypes}, severityObjs)
				resp.Diagnostics.Append(diags...)

				catObj, diags := types.ObjectValue(categoryCountAttrTypes, map[string]attr.Value{
					"category": types.StringValue(cat.Category),
					"counts":   severityList,
				})
				resp.Diagnostics.Append(diags...)
				categoryObjs = append(categoryObjs, catObj)
			}
		}
		list, diags := types.ListValue(types.ObjectType{AttrTypes: categoryCountAttrTypes}, categoryObjs)
		resp.Diagnostics.Append(diags...)
		return list
	}

	data.OpenInstanceCounts = setGrouped(result.OpenInstanceCounts)
	data.SuppressedInstanceCounts = setGrouped(result.SuppressedInstanceCounts)
	data.ResolvedInstanceCounts = setGrouped(result.ResolvedInstanceCounts)
	data.AcknowledgedInstanceCounts = setGrouped(result.AcknowledgedInstanceCounts)
	data.TotalInstanceCount = types.Int64Value(result.TotalInstanceCount)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
