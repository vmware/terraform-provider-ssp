// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ resource.Resource = &SiteIdpsConfigResource{}
var _ resource.ResourceWithImportState = &SiteIdpsConfigResource{}

func NewSiteIdpsConfigResource() resource.Resource {
	return &SiteIdpsConfigResource{}
}

// SiteIdpsConfigResource manages the per-site IDPS signature configuration.
// Unlike most config resources in this codebase, this one has a real
// DELETE API and is keyed by site_id rather than being a singleton.
type SiteIdpsConfigResource struct {
	client *client.Client
}

// SiteIdpsConfigResourceModel is the Terraform state model.
type SiteIdpsConfigResourceModel struct {
	ID              types.String `tfsdk:"id"`
	SiteID          types.String `tfsdk:"site_id"`
	AssignedVersion types.String `tfsdk:"assigned_version"`
	AutoUpdate      types.Bool   `tfsdk:"auto_update"`
}

func (r *SiteIdpsConfigResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_site_idps_config"
}

func (r *SiteIdpsConfigResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the IDPS signature configuration for a specific onboarded site.\n\n" +
			"Corresponds to `GET/PUT/POST/DELETE /ssp/security-content/feature-config/idps/{site-id}`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Unique identifier of the site IDPS configuration, as returned by the API.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"site_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Unique identifier of the onboarded site (e.g. `ssp_site.example.id`) this configuration applies to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"assigned_version": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "IDPS signature version assigned to this site.",
			},
			"auto_update": schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: "Whether this site automatically updates to the latest IDPS signature version.",
			},
		},
	}
}

func (r *SiteIdpsConfigResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func siteIdpsConfigPath(siteID string) string {
	return "/ssp/security-content/feature-config/idps/" + siteID
}

func (r *SiteIdpsConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SiteIdpsConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := data.SiteID.ValueString()
	payload := &client.SiteIdpsConfig{
		SiteID:          siteID,
		AssignedVersion: data.AssignedVersion.ValueString(),
		AutoUpdate:      data.AutoUpdate.ValueBool(),
	}

	var result client.SiteIdpsConfig
	if _, err := r.client.Post(ctx, siteIdpsConfigPath(siteID), payload, &result); err != nil {
		resp.Diagnostics.AddError("Error creating site IDPS config", err.Error())
		return
	}

	mapSiteIdpsConfigToState(&result, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SiteIdpsConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SiteIdpsConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result client.SiteIdpsConfig
	status, err := r.client.Get(ctx, siteIdpsConfigPath(data.SiteID.ValueString()), &result)
	if status == 404 {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading site IDPS config", err.Error())
		return
	}

	mapSiteIdpsConfigToState(&result, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SiteIdpsConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data SiteIdpsConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := data.SiteID.ValueString()

	var current client.SiteIdpsConfig
	if _, err := r.client.Get(ctx, siteIdpsConfigPath(siteID), &current); err != nil {
		resp.Diagnostics.AddError("Error reading current site IDPS config", err.Error())
		return
	}

	payload := &client.SiteIdpsConfig{
		Revision:        current.Revision,
		SiteID:          siteID,
		AssignedVersion: data.AssignedVersion.ValueString(),
		AutoUpdate:      data.AutoUpdate.ValueBool(),
	}

	var result client.SiteIdpsConfig
	if status, err := r.client.Put(ctx, siteIdpsConfigPath(siteID), payload, &result); err != nil {
		if status == 409 {
			resp.Diagnostics.AddError("Error updating site IDPS config",
				fmt.Sprintf("%s (409 means the configuration was modified since it was last read; retry the apply)", err.Error()))
			return
		}
		resp.Diagnostics.AddError("Error updating site IDPS config", err.Error())
		return
	}

	mapSiteIdpsConfigToState(&result, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete removes the site IDPS configuration. Unlike most config resources
// in this codebase, this one has a real DELETE API.
func (r *SiteIdpsConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SiteIdpsConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	status, err := r.client.Delete(ctx, siteIdpsConfigPath(data.SiteID.ValueString()))
	if status == 404 {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error deleting site IDPS config", err.Error())
	}
}

func (r *SiteIdpsConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), req.ID)...)
}

func mapSiteIdpsConfigToState(result *client.SiteIdpsConfig, m *SiteIdpsConfigResourceModel) {
	m.ID = types.StringValue(result.ID)
	m.SiteID = types.StringValue(result.SiteID)
	if result.AssignedVersion != "" {
		m.AssignedVersion = types.StringValue(result.AssignedVersion)
	} else {
		m.AssignedVersion = types.StringNull()
	}
	m.AutoUpdate = types.BoolValue(result.AutoUpdate)
}
