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

var _ resource.Resource = &IntelligenceConfigResource{}
var _ resource.ResourceWithImportState = &IntelligenceConfigResource{}

func NewIntelligenceConfigResource() resource.Resource {
	return &IntelligenceConfigResource{}
}

// IntelligenceConfigResource manages the singleton Security Intelligence
// pre-deployment configuration.
type IntelligenceConfigResource struct {
	client *client.Client
}

// ── Terraform model types ─────────────────────────────────────────────────────

type IntelligenceConfigResourceModel struct {
	ID                     types.String `tfsdk:"id"`
	EnableAdvancedFeatures types.Bool   `tfsdk:"enable_advanced_features"`
	ConfigStatus           types.String `tfsdk:"config_status"`
	ConfigMessage          types.String `tfsdk:"config_message"`
}

// ── Metadata / Schema ─────────────────────────────────────────────────────────

func (r *IntelligenceConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_intelligence_config"
}

func (r *IntelligenceConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the pre-deployment configuration for the **Security Intelligence** feature.\n\n" +
			"This configuration must be applied before deploying the `INTELLIGENCE`\n" +
			"`ssp_feature`. It is a singleton — only one configuration exists per SSP platform.\n\n" +
			"Use `terraform import ssp_intelligence_config.this singleton` to adopt\n" +
			"an existing configuration.\n\n" +
			"Corresponds to `GET/PUT /ssp/lcm/intelligence/config` and\n" +
			"`GET /ssp/lcm/intelligence/config/status`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Always `singleton` for this resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enable_advanced_features": schema.BoolAttribute{
				Required: true,
				MarkdownDescription: "Enables advanced features for Security Intelligence. " +
					"This may require adding additional worker nodes to Security Services Platform.",
			},
			"config_status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Configuration status from the platform.",
			},
			"config_message": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Informational message accompanying `config_status`.",
			},
		},
	}
}

// ── Configure ─────────────────────────────────────────────────────────────────

func (r *IntelligenceConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ── CRUD ──────────────────────────────────────────────────────────────────────

func (r *IntelligenceConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data IntelligenceConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.putConfig(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error creating intelligence config", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IntelligenceConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data IntelligenceConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.readConfig(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading intelligence config", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IntelligenceConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data IntelligenceConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.putConfig(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error updating intelligence config", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete removes the resource from Terraform state only. The SSP API does not
// support deleting the intelligence configuration; it is a persistent
// platform setting.
func (r *IntelligenceConfigResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *IntelligenceConfigResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var data IntelligenceConfigResourceModel

	if err := r.readConfig(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error importing intelligence config", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// putConfig fetches the current revision, then PUTs the desired configuration,
// and finally reads back the applied state (including config status).
func (r *IntelligenceConfigResource) putConfig(ctx context.Context, data *IntelligenceConfigResourceModel) error {
	// The revision-fetch-then-PUT sequence below races with any other SSP LCM
	// lifecycle action (feature/site/upgrade/backup/restore); serialize with them.
	lcmMu.Lock()
	defer lcmMu.Unlock()

	// Fetch the current revision to satisfy optimistic-locking.
	var current client.IntelligenceConfiguration
	if _, err := r.client.Get(ctx, "/ssp/lcm/intelligence/config", &current); err != nil {
		return fmt.Errorf("GET /ssp/lcm/intelligence/config: %w", err)
	}

	payload := &client.IntelligenceConfiguration{
		Revision:               current.Revision,
		EnableAdvancedFeatures: data.EnableAdvancedFeatures.ValueBool(),
	}

	if _, err := r.client.Put(ctx, "/ssp/lcm/intelligence/config", payload, nil); err != nil {
		return fmt.Errorf("PUT /ssp/lcm/intelligence/config: %w", err)
	}

	return r.readConfig(ctx, data)
}

// readConfig reads the current config and status from the API into data.
func (r *IntelligenceConfigResource) readConfig(ctx context.Context, data *IntelligenceConfigResourceModel) error {
	var cfg client.IntelligenceConfiguration
	if _, err := r.client.Get(ctx, "/ssp/lcm/intelligence/config", &cfg); err != nil {
		return fmt.Errorf("GET /ssp/lcm/intelligence/config: %w", err)
	}

	var status client.IntelligenceConfigurationStatus
	if _, err := r.client.Get(ctx, "/ssp/lcm/intelligence/config/status", &status); err != nil {
		return fmt.Errorf("GET /ssp/lcm/intelligence/config/status: %w", err)
	}

	data.ID = types.StringValue("singleton")
	data.EnableAdvancedFeatures = types.BoolValue(cfg.EnableAdvancedFeatures)
	data.ConfigStatus = types.StringValue(status.Status)
	data.ConfigMessage = types.StringValue(status.Message)
	return nil
}
