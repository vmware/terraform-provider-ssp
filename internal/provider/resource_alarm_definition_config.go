// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ resource.Resource = &AlarmDefinitionConfigResource{}
var _ resource.ResourceWithImportState = &AlarmDefinitionConfigResource{}

func NewAlarmDefinitionConfigResource() resource.Resource {
	return &AlarmDefinitionConfigResource{}
}

// AlarmDefinitionConfigResource manages the enabled state of a pre-existing,
// system-defined alarm definition. Alarm definitions cannot be created or
// deleted through the API (there is no POST/DELETE for
// /ssp/alarms/definitions/{id}), so this resource models "adopt and
// configure an existing definition by ID."
type AlarmDefinitionConfigResource struct {
	client *client.Client
}

type AlarmDefinitionConfigResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	Enabled            types.Bool   `tfsdk:"enabled"`
	FeatureName        types.String `tfsdk:"feature_name"`
	FeatureDisplayName types.String `tfsdk:"feature_display_name"`
	EventType          types.String `tfsdk:"event_type"`
	Severity           types.String `tfsdk:"severity"`
	Summary            types.String `tfsdk:"summary"`
	Description        types.String `tfsdk:"description"`
	RecommendedAction  types.String `tfsdk:"recommended_action"`
	KbArticle          types.String `tfsdk:"kb_article"`
}

func (r *AlarmDefinitionConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alarm_definition_config"
}

func (r *AlarmDefinitionConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the enabled state of a pre-existing, system-defined alarm definition.\n\n" +
			"Alarm definitions are created by the platform, not by this resource — there is no\n" +
			"`POST`/`DELETE` for `/ssp/alarms/definitions/{id}`. Use `id` to adopt an existing\n" +
			"definition (see `data.ssp_alarm_definitions` to discover valid IDs) and `enabled`\n" +
			"to control whether it can generate new alarm instances.\n\n" +
			"`Delete()` only removes this resource from Terraform state — there is no remote\n" +
			"call to undo, since the definition itself is not deletable.\n\n" +
			"Corresponds to `GET /ssp/alarms/definitions/{id}` and `POST /ssp/alarms/definitions/{id}`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the pre-existing alarm definition to configure.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"enabled": schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: "Whether this alarm definition can generate new alarm instances.",
			},
			"feature_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "System name of the feature this alarm is associated with.",
			},
			"feature_display_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Human-readable name of the feature this alarm belongs to.",
			},
			"event_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Machine-readable event type that triggers this alarm.",
			},
			"severity": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Severity level of this alarm (`CRITICAL`, `HIGH`, `MEDIUM`, `INFORMATIONAL`).",
			},
			"summary": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Brief overview of the alarm's purpose.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Full description of this alarm.",
			},
			"recommended_action": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Guidance on how to address and resolve the alarm condition.",
			},
			"kb_article": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "URL to a Knowledge Base article with more detail.",
			},
		},
	}
}

func (r *AlarmDefinitionConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, err := GetClientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected provider data type", err.Error())
		return
	}
	r.client = c
}

func (r *AlarmDefinitionConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data AlarmDefinitionConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.updateDefinition(ctx, data.ID.ValueString(), data.Enabled.ValueBool()); err != nil {
		resp.Diagnostics.AddError("Error configuring alarm definition", err.Error())
		return
	}

	if err := r.readDefinition(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading alarm definition after configuring it", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AlarmDefinitionConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data AlarmDefinitionConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.readDefinition(ctx, &data); err != nil {
		if err == errAlarmDefinitionNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading alarm definition", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AlarmDefinitionConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data AlarmDefinitionConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.updateDefinition(ctx, data.ID.ValueString(), data.Enabled.ValueBool()); err != nil {
		resp.Diagnostics.AddError("Error configuring alarm definition", err.Error())
		return
	}

	if err := r.readDefinition(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading alarm definition after configuring it", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete removes the resource from Terraform state only: alarm definitions
// are platform-defined and cannot be deleted via the API.
func (r *AlarmDefinitionConfigResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *AlarmDefinitionConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var data AlarmDefinitionConfigResourceModel
	data.ID = types.StringValue(req.ID)
	if err := r.readDefinition(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error importing alarm definition", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

var errAlarmDefinitionNotFound = fmt.Errorf("alarm definition not found")

func (r *AlarmDefinitionConfigResource) updateDefinition(ctx context.Context, id string, enabled bool) error {
	_, err := r.client.Post(ctx, "/ssp/alarms/definitions/"+id, &client.UpdateAlarmDefinitionRequest{Enabled: enabled}, nil)
	if err != nil {
		return fmt.Errorf("POST /ssp/alarms/definitions/%s: %w", id, err)
	}
	return nil
}

// readDefinition fetches GET /ssp/alarms/definitions/{id}, which — unlike a
// typical single-item GET — returns an AlarmDefinitionListResult wrapper
// rather than a bare AlarmDefinition (confirmed against the spec's
// GetAlarmDefinitionsById response schema).
func (r *AlarmDefinitionConfigResource) readDefinition(ctx context.Context, data *AlarmDefinitionConfigResourceModel) error {
	id := data.ID.ValueString()
	var result client.AlarmDefinitionListResult
	httpStatus, err := r.client.Get(ctx, "/ssp/alarms/definitions/"+id, &result)
	if httpStatus == 404 {
		return errAlarmDefinitionNotFound
	}
	if err != nil {
		return fmt.Errorf("GET /ssp/alarms/definitions/%s: %w", id, err)
	}
	if len(result.Results) == 0 {
		return errAlarmDefinitionNotFound
	}

	def := result.Results[0]
	data.ID = types.StringValue(id)
	data.Enabled = types.BoolValue(def.Enabled)
	data.FeatureName = types.StringValue(def.FeatureName)
	data.FeatureDisplayName = types.StringValue(def.FeatureDisplayName)
	data.EventType = types.StringValue(def.EventType)
	data.Severity = types.StringValue(def.Severity)
	data.Summary = types.StringValue(def.Summary)
	data.Description = types.StringValue(def.Description)
	data.RecommendedAction = types.StringValue(def.RecommendedAction)
	data.KbArticle = types.StringValue(def.KbArticle)
	return nil
}
