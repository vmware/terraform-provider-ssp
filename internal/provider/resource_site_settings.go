// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ resource.Resource = &SiteSettingsResource{}
var _ resource.ResourceWithImportState = &SiteSettingsResource{}

func NewSiteSettingsResource() resource.Resource {
	return &SiteSettingsResource{}
}

// SiteSettingsResource manages the per-site SSP feature settings that are
// created automatically during a site's onboarding workflow. There is no
// POST/DELETE for this object — "create" in Terraform terms means "start
// managing an already-existing settings object", and "delete" is a no-op.
type SiteSettingsResource struct {
	client *client.Client
}

type SiteSettingsResourceModel struct {
	ID                    types.String `tfsdk:"id"`
	SiteID                types.String `tfsdk:"site_id"`
	IsIntelligenceEnabled types.Bool   `tfsdk:"is_intelligence_enabled"`
	IsMetricsEnabled      types.Bool   `tfsdk:"is_metrics_enabled"`
	IsMpsEnabled          types.Bool   `tfsdk:"is_mps_enabled"`
	IsNdrEnabled          types.Bool   `tfsdk:"is_ndr_enabled"`
	IsRuleAnalysisEnabled types.Bool   `tfsdk:"is_rule_analysis_enabled"`
}

func (r *SiteSettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_site_settings"
}

func (r *SiteSettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the per-site SSP feature settings for an already-onboarded site.\n\n" +
			"These settings are created automatically as part of the site onboarding workflow " +
			"(`ssp_site`) — this resource does not create or delete them, only configures the " +
			"subset that is editable via API. Configuring a setting here does not activate or " +
			"deploy the corresponding feature; it only adjusts that feature's per-site behavior.\n\n" +
			"Corresponds to `GET/PUT /ssp/sites/{site-id}/settings`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the site settings object, as returned by the API.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"site_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the onboarded site (`ssp_site.id`) these settings belong to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"is_intelligence_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Site-specific setting for the Intelligence feature.",
			},
			"is_metrics_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Site-specific setting for the Metrics feature.",
			},
			"is_mps_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Site-specific setting for the Malware Prevention Service feature.",
			},
			"is_ndr_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Site-specific setting for the NDR feature.",
			},
			"is_rule_analysis_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Site-specific setting for the Rule Analysis feature.",
			},
		},
	}
}

func (r *SiteSettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SiteSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SiteSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := data.SiteID.ValueString()

	var current client.SiteFeatureSettings
	if _, err := r.client.Get(ctx, "/ssp/sites/"+siteID+"/settings", &current); err != nil {
		resp.Diagnostics.AddError("Error reading current site settings", err.Error())
		return
	}

	payload := buildSiteSettingsPayload(&data, &current)
	payload.Revision = current.Revision

	var result client.SiteFeatureSettings
	if _, err := r.client.Put(ctx, "/ssp/sites/"+siteID+"/settings", payload, &result); err != nil {
		resp.Diagnostics.AddError("Error updating site settings", err.Error())
		return
	}

	mapSiteSettingsToState(&result, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SiteSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SiteSettingsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := data.SiteID.ValueString()
	var result client.SiteFeatureSettings
	httpStatus, err := r.client.Get(ctx, "/ssp/sites/"+siteID+"/settings", &result)
	if httpStatus == 404 {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading site settings", err.Error())
		return
	}

	mapSiteSettingsToState(&result, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SiteSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data SiteSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := data.SiteID.ValueString()
	var current client.SiteFeatureSettings
	if _, err := r.client.Get(ctx, "/ssp/sites/"+siteID+"/settings", &current); err != nil {
		resp.Diagnostics.AddError("Error reading current site settings", err.Error())
		return
	}

	payload := buildSiteSettingsPayload(&data, &current)
	payload.Revision = current.Revision

	var result client.SiteFeatureSettings
	if _, err := r.client.Put(ctx, "/ssp/sites/"+siteID+"/settings", payload, &result); err != nil {
		resp.Diagnostics.AddError("Error updating site settings", err.Error())
		return
	}

	mapSiteSettingsToState(&result, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete removes the resource from Terraform state only. Site feature
// settings have no DELETE API — they are tied to the site's own lifecycle
// (created on onboarding, removed on offboarding), not to this resource.
func (r *SiteSettingsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
}

func (r *SiteSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("site_id"), req, resp)
}

// buildSiteSettingsPayload starts from the current server state (so any
// Optional+Computed attribute the user didn't set keeps its current value)
// and overlays whichever attributes are actually set in the plan.
func buildSiteSettingsPayload(m *SiteSettingsResourceModel, current *client.SiteFeatureSettings) *client.SiteFeatureSettings {
	payload := &client.SiteFeatureSettings{
		SiteIDs:               current.SiteIDs,
		IsIntelligenceEnabled: current.IsIntelligenceEnabled,
		IsMetricsEnabled:      current.IsMetricsEnabled,
		IsMpsEnabled:          current.IsMpsEnabled,
		IsNdrEnabled:          current.IsNdrEnabled,
		IsRuleAnalysisEnabled: current.IsRuleAnalysisEnabled,
	}
	if !m.IsIntelligenceEnabled.IsNull() && !m.IsIntelligenceEnabled.IsUnknown() {
		payload.IsIntelligenceEnabled = m.IsIntelligenceEnabled.ValueBool()
	}
	if !m.IsMetricsEnabled.IsNull() && !m.IsMetricsEnabled.IsUnknown() {
		payload.IsMetricsEnabled = m.IsMetricsEnabled.ValueBool()
	}
	if !m.IsMpsEnabled.IsNull() && !m.IsMpsEnabled.IsUnknown() {
		payload.IsMpsEnabled = m.IsMpsEnabled.ValueBool()
	}
	if !m.IsNdrEnabled.IsNull() && !m.IsNdrEnabled.IsUnknown() {
		payload.IsNdrEnabled = m.IsNdrEnabled.ValueBool()
	}
	if !m.IsRuleAnalysisEnabled.IsNull() && !m.IsRuleAnalysisEnabled.IsUnknown() {
		payload.IsRuleAnalysisEnabled = m.IsRuleAnalysisEnabled.ValueBool()
	}
	return payload
}

func mapSiteSettingsToState(result *client.SiteFeatureSettings, m *SiteSettingsResourceModel) {
	m.ID = types.StringValue(result.ID)
	m.IsIntelligenceEnabled = types.BoolValue(result.IsIntelligenceEnabled)
	m.IsMetricsEnabled = types.BoolValue(result.IsMetricsEnabled)
	m.IsMpsEnabled = types.BoolValue(result.IsMpsEnabled)
	m.IsNdrEnabled = types.BoolValue(result.IsNdrEnabled)
	m.IsRuleAnalysisEnabled = types.BoolValue(result.IsRuleAnalysisEnabled)
}
