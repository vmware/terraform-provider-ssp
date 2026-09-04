package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ resource.Resource = &SiteResource{}
var _ resource.ResourceWithImportState = &SiteResource{}

func NewSiteResource() resource.Resource {
	return &SiteResource{}
}

// SiteResource manages the onboarding of an NSX Manager (or AVI/SSP) site with SSP.
type SiteResource struct {
	client *client.Client
}

// SiteResourceModel is the Terraform state model.
type SiteResourceModel struct {
	ID               types.String `tfsdk:"id"`
	SiteType         types.String `tfsdk:"site_type"`
	SiteName         types.String `tfsdk:"site_name"`
	DesiredState     types.String `tfsdk:"desired_state"`
	Force            types.Bool   `tfsdk:"force"`
	CurrentState     types.String `tfsdk:"current_state"`
	ConnectionStatus types.String `tfsdk:"connection_status"`
	NsxVersion       types.String `tfsdk:"nsx_version"`
	NsxClusterID     types.String `tfsdk:"nsx_cluster_id"`
	// site_connection_info holds write-only credentials; stored sensitive in state.
	SiteConnectionInfo types.Object `tfsdk:"site_connection_info"`
}

var siteConnectionInfoAttrTypes = map[string]attr.Type{
	"connection_type": types.StringType,
	"hostname":        types.StringType,
	"host_addresses":  types.ListType{ElemType: types.StringType},
	"username":        types.StringType,
	"password":        types.StringType,
	"certificate":     types.StringType,
}

func (r *SiteResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_site"
}

func (r *SiteResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Onboards a site (NSX Manager, AVI, or SSP) to the SSP platform.\n\n" +
			"The onboarding workflow is asynchronous; the provider polls until the site reaches\n" +
			"READY state (up to 90 minutes).\n\n" +
			"Credentials in `site_connection_info` are **one-time use** — SSP never stores or\n" +
			"returns them. They are kept in Terraform state (encrypted) so that reconnect and\n" +
			"offboard operations can use them.\n\n" +
			"To adopt an existing site: `terraform import ssp_site.this <site-id>`.\n\n" +
			"Corresponds to `POST/GET/PUT/DELETE /ssp/site-service/sites`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Unique UUID of the site assigned by SSP.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"site_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Site type: `NSX_MANAGER`, `AVI`, or `SSP`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"site_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the site. For NSX_MANAGER sites this must match the name configured on NSX and is immutable after creation.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"desired_state": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Desired onboarding state: `ONBOARD` (full onboard), `PREPARE` (prechecks only), or `OFFBOARD`.",
			},
			"force": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "When `true`, force-onboards the site even if references to another SSP instance remain on it.",
			},
			"current_state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current readiness state of the site: `READY`, `NOT_READY`, `INACTIVE`, or `UNKNOWN`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"connection_status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current connection status between SSP and the site: `HEALTHY`, `UNHEALTHY`, or `UNKNOWN`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"nsx_version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "NSX software version reported by the site (NSX_MANAGER sites only).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"nsx_cluster_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "NSX cluster ID of the site (NSX_MANAGER sites only).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"site_connection_info": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: "Connection details for the site. Credentials are write-only (never returned by the API).",
				Sensitive:           true,
				PlanModifiers:       []planmodifier.Object{objectplanmodifier.UseStateForUnknown()},
				Attributes: map[string]schema.Attribute{
					"connection_type": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Connection method: `DYNAMIC` (single hostname) or `STATIC` (list of addresses).",
					},
					"hostname": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Hostname or IP of the site. Required for `DYNAMIC` connections.",
					},
					"host_addresses": schema.ListAttribute{
						Optional:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "List of host addresses (IP or FQDN, optionally with port). Required for `STATIC` connections.",
					},
					"username": schema.StringAttribute{
						Required:            true,
						Sensitive:           true,
						MarkdownDescription: "Username for the site. Required for onboard and reconnect.",
					},
					"password": schema.StringAttribute{
						Required:            true,
						Sensitive:           true,
						MarkdownDescription: "Password for the site. Required for onboard, reconnect, and offboard.",
					},
					"certificate": schema.StringAttribute{
						Required:  true,
						Sensitive: true,
						MarkdownDescription: "PEM-encoded TLS certificate for the site. Required for all operations " +
							"— confirmed live: the API rejects onboard/reconnect without it " +
							"(`\"Username, Password, Certificate and DiscoveryHostname are required\"`). " +
							"Previously marked Optional, which let this requirement surface only as a runtime 400.",
					},
				},
			},
		},
	}
}

func (r *SiteResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SiteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SiteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	lcmMu.Lock()
	defer lcmMu.Unlock()

	conn, d := extractSiteConnectionInfo(ctx, data.SiteConnectionInfo)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := &client.Site{
		SiteType:           data.SiteType.ValueString(),
		SiteName:           data.SiteName.ValueString(),
		DesiredState:       data.DesiredState.ValueString(),
		SiteConnectionInfo: conn,
	}

	path := "/ssp/site-service/sites"
	if data.Force.ValueBool() {
		path += "?force=true"
	}

	var asyncResp client.AsyncApiResponse
	_, err := r.client.Post(ctx, path, payload, &asyncResp)
	if err != nil {
		resp.Diagnostics.AddError("Error creating site", err.Error())
		return
	}

	siteID := asyncResp.ID
	if siteID == "" {
		resp.Diagnostics.AddError("No site ID returned", "The API returned an empty ID after site creation.")
		return
	}

	if err := r.client.WaitForSiteReady(ctx, siteID); err != nil {
		resp.Diagnostics.AddError("Error waiting for site to become ready", err.Error())
		return
	}

	var site client.Site
	if _, err := r.client.Get(ctx, "/ssp/site-service/sites/"+siteID, &site); err != nil {
		resp.Diagnostics.AddError("Error reading site after create", err.Error())
		return
	}

	mapSiteToState(&site, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SiteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SiteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var site client.Site
	status, err := r.client.Get(ctx, "/ssp/site-service/sites/"+data.ID.ValueString(), &site)
	if status == 404 {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading site", err.Error())
		return
	}

	mapSiteToState(&site, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SiteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data SiteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state SiteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	lcmMu.Lock()
	defer lcmMu.Unlock()

	conn, d := extractSiteConnectionInfo(ctx, data.SiteConnectionInfo)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	var current client.Site
	if _, err := r.client.Get(ctx, "/ssp/site-service/sites/"+state.ID.ValueString(), &current); err != nil {
		resp.Diagnostics.AddError("Error reading current site before update", err.Error())
		return
	}

	payload := &client.Site{
		Revision:           current.Revision,
		SiteType:           data.SiteType.ValueString(),
		SiteName:           data.SiteName.ValueString(),
		DesiredState:       data.DesiredState.ValueString(),
		SiteConnectionInfo: conn,
	}

	var asyncResp client.AsyncApiResponse
	_, err := r.client.Put(ctx, "/ssp/site-service/sites/"+state.ID.ValueString(), payload, &asyncResp)
	if err != nil {
		resp.Diagnostics.AddError("Error updating site", err.Error())
		return
	}

	if err := r.client.WaitForSiteReady(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error waiting for site update to complete", err.Error())
		return
	}

	var site client.Site
	if _, err := r.client.Get(ctx, "/ssp/site-service/sites/"+state.ID.ValueString(), &site); err != nil {
		resp.Diagnostics.AddError("Error reading site after update", err.Error())
		return
	}

	mapSiteToState(&site, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SiteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SiteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	lcmMu.Lock()
	defer lcmMu.Unlock()

	conn, d := extractSiteConnectionInfo(ctx, data.SiteConnectionInfo)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	creds := &client.UserCredentials{
		Username: conn.Username,
		Password: conn.Password,
	}

	path := "/ssp/site-service/sites/" + data.ID.ValueString()
	if data.Force.ValueBool() {
		path += "?force=true"
	}
	status, err := r.client.DeleteWithBody(ctx, path, creds)
	if status == 404 {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error deleting site", err.Error())
		return
	}

	if err := r.client.WaitForSiteDeleted(ctx, data.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error waiting for site deletion", err.Error())
		return
	}
}

func (r *SiteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var data SiteResourceModel
	data.ID = types.StringValue(req.ID)

	var site client.Site
	status, err := r.client.Get(ctx, "/ssp/site-service/sites/"+req.ID, &site)
	if status == 404 {
		resp.Diagnostics.AddError("Site not found", fmt.Sprintf("No site with ID %s", req.ID))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error importing site", err.Error())
		return
	}

	mapSiteToState(&site, &data)

	// Credentials (username/password/certificate) are write-only and never echoed
	// back by the API, so those are populated with empty placeholders -- the user
	// must supply real values after import. connection_type and hostname/
	// host_addresses, however, ARE returned by the API when present, so use the
	// real values instead of assuming DYNAMIC: importing a STATIC site with a
	// hardcoded "DYNAMIC" would send the wrong shape of payload on the next apply.
	connectionType := "DYNAMIC"
	hostname := ""
	hostAddresses := []attr.Value{}
	if site.SiteConnectionInfo != nil {
		if site.SiteConnectionInfo.ConnectionType != "" {
			connectionType = site.SiteConnectionInfo.ConnectionType
		}
		hostname = site.SiteConnectionInfo.Hostname
		for _, addr := range site.SiteConnectionInfo.HostAddresses {
			hostAddresses = append(hostAddresses, types.StringValue(addr))
		}
	}

	if data.SiteConnectionInfo.IsNull() || data.SiteConnectionInfo.IsUnknown() {
		connObj, diags := types.ObjectValue(siteConnectionInfoAttrTypes, map[string]attr.Value{
			"connection_type": types.StringValue(connectionType),
			"hostname":        types.StringValue(hostname),
			"host_addresses":  types.ListValueMust(types.StringType, hostAddresses),
			"username":        types.StringValue(""),
			"password":        types.StringValue(""),
			"certificate":     types.StringValue(""),
		})
		resp.Diagnostics.Append(diags...)
		data.SiteConnectionInfo = connObj
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// extractSiteConnectionInfo deserialises the site_connection_info object from state.
func extractSiteConnectionInfo(ctx context.Context, obj types.Object) (*client.SiteConnection, diag.Diagnostics) {
	var diags diag.Diagnostics
	if obj.IsNull() || obj.IsUnknown() {
		return nil, diags
	}

	var raw struct {
		ConnectionType string       `tfsdk:"connection_type"`
		Hostname       types.String `tfsdk:"hostname"`
		HostAddresses  types.List   `tfsdk:"host_addresses"`
		Username       types.String `tfsdk:"username"`
		Password       types.String `tfsdk:"password"`
		Certificate    types.String `tfsdk:"certificate"`
	}
	diags.Append(obj.As(ctx, &raw, objectAsOptions)...)
	if diags.HasError() {
		return nil, diags
	}

	conn := &client.SiteConnection{
		ConnectionType: raw.ConnectionType,
		Username:       raw.Username.ValueString(),
		Password:       raw.Password.ValueString(),
		Certificate:    raw.Certificate.ValueString(),
	}

	if raw.ConnectionType == "DYNAMIC" {
		conn.Hostname = raw.Hostname.ValueString()
	} else {
		var addrs []string
		diags.Append(raw.HostAddresses.ElementsAs(ctx, &addrs, false)...)
		conn.HostAddresses = addrs
	}

	return conn, diags
}

// mapSiteToState copies the API site response into the Terraform state model.
// Credentials in site_connection_info are preserved from existing state (never overwritten).
func mapSiteToState(site *client.Site, m *SiteResourceModel) {
	m.ID = types.StringValue(site.ID)
	m.SiteType = types.StringValue(site.SiteType)
	m.SiteName = types.StringValue(site.SiteName)
	m.CurrentState = types.StringValue(site.CurrentState)

	if site.Status != nil {
		m.ConnectionStatus = types.StringValue(site.Status.ConnectionStatus)
		m.NsxVersion = types.StringValue(site.Status.NsxVersion)
		m.NsxClusterID = types.StringValue(site.Status.NsxClusterID)
	} else {
		m.ConnectionStatus = types.StringValue("")
		m.NsxVersion = types.StringValue("")
		m.NsxClusterID = types.StringValue("")
	}
	// site_connection_info: credentials are never returned by the API; preserve from state.
}
