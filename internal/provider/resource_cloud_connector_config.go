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

var _ resource.Resource = &CloudConnectorConfigResource{}
var _ resource.ResourceWithImportState = &CloudConnectorConfigResource{}

func NewCloudConnectorConfigResource() resource.Resource {
	return &CloudConnectorConfigResource{}
}

// CloudConnectorConfigResource manages the singleton Cloud Connector
// pre-deployment configuration.
type CloudConnectorConfigResource struct {
	client *client.Client
}

// ── Terraform model types ─────────────────────────────────────────────────────

type CloudConnectorConfigResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Region        types.String `tfsdk:"region"`
	FQDN          types.String `tfsdk:"fqdn"`
	RegionName    types.String `tfsdk:"region_name"`
	Configurable  types.Bool   `tfsdk:"configurable"`
	ConfigStatus  types.String `tfsdk:"config_status"`
	ConfigMessage types.String `tfsdk:"config_message"`
}

// ── Metadata / Schema ─────────────────────────────────────────────────────────

func (r *CloudConnectorConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cloud_connector_config"
}

func (r *CloudConnectorConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the pre-deployment configuration for the **Cloud Connector** feature.\n\n" +
			"Cloud Connector is a prerequisite for features that require cloud connectivity,\n" +
			"such as **Malware Prevention** in `CLOUD` analysis mode. The configuration\n" +
			"selects the cloud region where analysis traffic is sent.\n\n" +
			"This is a singleton — only one configuration exists per SSP platform.\n\n" +
			"Once Cloud Connector has been deployed, the region cannot be changed until\n" +
			"the feature is removed and re-deployed (`configurable` becomes `false`).\n\n" +
			"Use `terraform import ssp_cloud_connector_config.this singleton` to adopt\n" +
			"an existing configuration.\n\n" +
			"Corresponds to `GET/PUT /ssp/lcm/cloud-connector/config` and\n" +
			"`GET /ssp/lcm/cloud-connector/config/status`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Always `singleton` for this resource.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"region": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Cloud Connector region identifier (e.g. `west.us` for *United States 1*, " +
					"`west.eu` for *European Union 1*). " +
					"Use `GET /ssp/lcm/cloud-connector/regions` to retrieve the list of available regions.",
			},
			"fqdn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "FQDN of the selected Cloud Connector region. Resolved automatically from `region`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"region_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Human-readable name of the selected region (e.g. `US 1`, `EU 1`). Resolved automatically from `region`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"configurable": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the region can still be changed. Becomes `false` once Cloud Connector is deployed.",
			},
			"config_status": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "Configuration status from the platform. One of `NOT_CONFIGURED`, " +
					"`DEPLOYMENT_REQUIRED`, or `CONFIGURATION_APPLIED`.",
			},
			"config_message": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Informational message accompanying `config_status`.",
			},
		},
	}
}

// ── Configure ─────────────────────────────────────────────────────────────────

func (r *CloudConnectorConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *CloudConnectorConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CloudConnectorConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.putConfig(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error creating cloud connector config", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CloudConnectorConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CloudConnectorConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.readConfig(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading cloud connector config", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CloudConnectorConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data CloudConnectorConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.putConfig(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error updating cloud connector config", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete removes the resource from Terraform state only. The SSP API does not
// support deleting the Cloud Connector configuration.
func (r *CloudConnectorConfigResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *CloudConnectorConfigResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var data CloudConnectorConfigResourceModel

	if err := r.readConfig(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error importing cloud connector config", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// putConfig resolves the region details from the available-regions API,
// fetches the current revision, PUTs the desired configuration, and finally
// reads back the applied state.
func (r *CloudConnectorConfigResource) putConfig(ctx context.Context, data *CloudConnectorConfigResourceModel) error {
	// The revision-fetch-then-PUT sequence below races with any other SSP LCM
	// lifecycle action (feature/site/upgrade/backup/restore); serialize with them.
	lcmMu.Lock()
	defer lcmMu.Unlock()

	regionID := data.Region.ValueString()

	// Resolve FQDN and region_name for the requested region identifier.
	var available client.AvailableCloudConnectorRegions
	if _, err := r.client.Get(ctx, "/ssp/lcm/cloud-connector/regions", &available); err != nil {
		return fmt.Errorf("GET /ssp/lcm/cloud-connector/regions: %w", err)
	}

	var resolved *client.CloudConnectorRegion
	for i := range available.CloudRegions {
		if available.CloudRegions[i].Region == regionID {
			resolved = &available.CloudRegions[i]
			break
		}
	}
	if resolved == nil {
		var names []string
		for _, r := range available.CloudRegions {
			names = append(names, fmt.Sprintf("%q (%s)", r.Region, r.RegionName))
		}
		return fmt.Errorf("region %q not found in available regions; available: %v", regionID, names)
	}

	// Fetch the current revision to satisfy optimistic-locking.
	var current client.CloudConnectorConfiguration
	if _, err := r.client.Get(ctx, "/ssp/lcm/cloud-connector/config", &current); err != nil {
		return fmt.Errorf("GET /ssp/lcm/cloud-connector/config: %w", err)
	}

	payload := &client.CloudConnectorConfiguration{
		Revision:   current.Revision,
		FQDN:       resolved.FQDN,
		Region:     resolved.Region,
		RegionName: resolved.RegionName,
	}

	// The PUT returns 202; pass nil as out to discard the async response body.
	if _, err := r.client.Put(ctx, "/ssp/lcm/cloud-connector/config", payload, nil); err != nil {
		return fmt.Errorf("PUT /ssp/lcm/cloud-connector/config: %w", err)
	}

	return r.readConfig(ctx, data)
}

// readConfig reads the current config and status from the API into data.
func (r *CloudConnectorConfigResource) readConfig(ctx context.Context, data *CloudConnectorConfigResourceModel) error {
	var cfg client.CloudConnectorConfiguration
	if _, err := r.client.Get(ctx, "/ssp/lcm/cloud-connector/config", &cfg); err != nil {
		return fmt.Errorf("GET /ssp/lcm/cloud-connector/config: %w", err)
	}

	var status client.CloudConnectorConfigurationStatus
	if _, err := r.client.Get(ctx, "/ssp/lcm/cloud-connector/config/status", &status); err != nil {
		return fmt.Errorf("GET /ssp/lcm/cloud-connector/config/status: %w", err)
	}

	data.ID = types.StringValue("singleton")
	data.Region = types.StringValue(cfg.Region)
	data.FQDN = types.StringValue(cfg.FQDN)
	data.RegionName = types.StringValue(cfg.RegionName)
	if cfg.Configurable != nil {
		data.Configurable = types.BoolValue(*cfg.Configurable)
	} else {
		data.Configurable = types.BoolValue(true)
	}
	data.ConfigStatus = types.StringValue(status.Status)
	data.ConfigMessage = types.StringValue(status.Message)

	return nil
}
