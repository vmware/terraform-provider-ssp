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

var _ datasource.DataSource = &AlarmDataSource{}

func NewAlarmDataSource() datasource.DataSource {
	return &AlarmDataSource{}
}

type AlarmDataSource struct {
	client *client.Client
}

type AlarmDataSourceModel struct {
	ID                types.String `tfsdk:"id"`
	DefinitionID      types.String `tfsdk:"definition_id"`
	FeatureName       types.String `tfsdk:"feature_name"`
	EventType         types.String `tfsdk:"event_type"`
	Severity          types.String `tfsdk:"severity"`
	Summary           types.String `tfsdk:"summary"`
	Description       types.String `tfsdk:"description"`
	RecommendedAction types.String `tfsdk:"recommended_action"`
	KbArticle         types.String `tfsdk:"kb_article"`
	ResourceID        types.String `tfsdk:"resource_id"`
	ObjectID          types.String `tfsdk:"object_id"`
	NodeID            types.String `tfsdk:"node_id"`
	Value             types.String `tfsdk:"value"`
	State             types.String `tfsdk:"state"`
	SuppressDuration  types.Int64  `tfsdk:"suppress_duration"`
}

func (d *AlarmDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alarm"
}

func (d *AlarmDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single alarm instance by ID.\n\n" +
			"Corresponds to `GET /ssp/alarms/{id}`.",
		Attributes: map[string]schema.Attribute{
			"id":                 schema.StringAttribute{Required: true, MarkdownDescription: "ID of the alarm instance."},
			"definition_id":      schema.StringAttribute{Computed: true, MarkdownDescription: "ID of this alarm's definition."},
			"feature_name":       schema.StringAttribute{Computed: true, MarkdownDescription: "Feature this alarm instance is associated with."},
			"event_type":         schema.StringAttribute{Computed: true, MarkdownDescription: "Event type that triggered this alarm instance."},
			"severity":           schema.StringAttribute{Computed: true, MarkdownDescription: "Severity level of this alarm instance."},
			"summary":            schema.StringAttribute{Computed: true, MarkdownDescription: "Brief overview of this alarm instance."},
			"description":        schema.StringAttribute{Computed: true, MarkdownDescription: "Full description of this alarm instance."},
			"recommended_action": schema.StringAttribute{Computed: true, MarkdownDescription: "Guidance on how to address this alarm instance."},
			"kb_article":         schema.StringAttribute{Computed: true, MarkdownDescription: "Link to a Knowledge Base article for this alarm."},
			"resource_id":        schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier of the resource this alarm was raised for."},
			"object_id":          schema.StringAttribute{Computed: true, MarkdownDescription: "Sub-object ID within the resource, if any."},
			"node_id":            schema.StringAttribute{Computed: true, MarkdownDescription: "UUID of the node that reported the value."},
			"value":              schema.StringAttribute{Computed: true, MarkdownDescription: "Current reported value associated with the alarm."},
			"state":              schema.StringAttribute{Computed: true, MarkdownDescription: "Current state of the alarm instance."},
			"suppress_duration":  schema.Int64Attribute{Computed: true, MarkdownDescription: "Hours the alarm is suppressed for, if `state` is `SUPPRESSED`."},
		},
	}
}

func (d *AlarmDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *AlarmDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data AlarmDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := data.ID.ValueString()
	var instance client.AlarmInstance
	_, err := d.client.Get(ctx, "/ssp/alarms/"+id, &instance)
	if err != nil {
		resp.Diagnostics.AddError("Error reading alarm instance "+id, err.Error())
		return
	}

	data.DefinitionID = types.StringValue(instance.DefinitionID)
	data.FeatureName = types.StringValue(instance.FeatureName)
	data.EventType = types.StringValue(instance.EventType)
	data.Severity = types.StringValue(instance.Severity)
	data.Summary = types.StringValue(instance.Summary)
	data.Description = types.StringValue(instance.Description)
	data.RecommendedAction = types.StringValue(instance.RecommendedAction)
	data.KbArticle = types.StringValue(instance.KbArticle)
	data.ResourceID = types.StringValue(instance.ResourceID)
	data.ObjectID = types.StringValue(instance.ObjectID)
	data.NodeID = types.StringValue(instance.NodeID)
	data.Value = types.StringValue(instance.Value)
	data.State = types.StringValue(instance.State)
	data.SuppressDuration = types.Int64Value(int64(instance.SuppressDuration))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
