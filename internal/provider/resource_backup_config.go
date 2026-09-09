// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ resource.Resource = &BackupConfigResource{}
var _ resource.ResourceWithImportState = &BackupConfigResource{}

func NewBackupConfigResource() resource.Resource {
	return &BackupConfigResource{}
}

// BackupConfigResource manages the singleton SFTP backup target configuration.
type BackupConfigResource struct {
	client *client.Client
}

// BackupConfigResourceModel is the Terraform state model for backup configuration.
type BackupConfigResourceModel struct {
	ID             types.String `tfsdk:"id"`
	ServerAddress  types.String `tfsdk:"server_address"`
	Protocol       types.String `tfsdk:"protocol"`
	Port           types.Int64  `tfsdk:"port"`
	Username       types.String `tfsdk:"username"`
	SSHPublicKey   types.String `tfsdk:"ssh_public_key"`
	BackupLocation types.String `tfsdk:"backup_location"`
	Password       types.String `tfsdk:"password"`
	Passphrase     types.String `tfsdk:"passphrase"`
}

func (r *BackupConfigResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_backup_config"
}

func (r *BackupConfigResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the remote backup server (SFTP) configuration for the SSP platform.\n\n" +
			"This is a singleton resource — only one backup configuration can exist per SSP instance.\n" +
			"Use `terraform import ssp_backup_config.this singleton` to adopt an existing configuration.\n\n" +
			"Corresponds to `GET/PUT /ssp/backup/config`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Always `singleton` for this resource.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"server_address": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "FQDN or IP address of the SFTP backup server (max 255 chars).",
			},
			"protocol": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "File transfer protocol. Currently only `SFTP` is supported.",
			},
			"port": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "TCP port for the SFTP connection (1–65535).",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"username": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "SSH username for the backup server (3–30 chars).",
			},
			"ssh_public_key": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "SSH public key of the backup server for host verification.",
			},
			"backup_location": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Directory on the backup server where backup bundles are stored. " +
					"Must not end in a trailing slash (rejected at plan time) — the API strips one if present, " +
					"which would otherwise cause a permanent post-apply diff.",
				Validators: []validator.String{noTrailingSlash()},
			},
			"password": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "Password for SSH authentication to the backup server (6–255 chars). Write-only.",
			},
			"passphrase": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
				MarkdownDescription: "Passphrase to encrypt backup bundles (6–255 chars). Write-only. " +
					"Marked optional in `apis/ssp_public_apis.yaml`'s `BackupConfig` schema, but the live " +
					"`PUT /ssp/backup/config` API rejects requests with an empty passphrase " +
					"(`\"Passphrase can not be empty.\"`) — required here to match observed live behavior.",
			},
		},
	}
}

func (r *BackupConfigResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *BackupConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data BackupConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := buildSspBackupConfigPayload(&data)

	// Fetch the current revision (if a backup config already exists) so this PUT
	// isn't rejected by the backend's optimistic-locking check; a 204 means no
	// backup config exists yet, so the zero-value Revision is correct as-is.
	var current client.BackupConfig
	status, err := r.client.Get(ctx, "/ssp/backup/config", &current)
	if status != 204 {
		if err != nil {
			resp.Diagnostics.AddError("Error reading current backup config", err.Error())
			return
		}
		payload.Revision = current.Revision
	}

	var result client.BackupConfig
	_, err = r.client.Put(ctx, "/ssp/backup/config", payload, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error creating backup config", err.Error())
		return
	}

	mapSspBackupConfigToState(&result, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BackupConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data BackupConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result client.BackupConfig
	status, err := r.client.Get(ctx, "/ssp/backup/config", &result)
	if status == 204 {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading backup config", err.Error())
		return
	}

	mapSspBackupConfigToState(&result, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BackupConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data BackupConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var current client.BackupConfig
	if _, err := r.client.Get(ctx, "/ssp/backup/config", &current); err != nil {
		resp.Diagnostics.AddError("Error reading current backup config", err.Error())
		return
	}

	payload := buildSspBackupConfigPayload(&data)
	payload.Revision = current.Revision
	var result client.BackupConfig
	_, err := r.client.Put(ctx, "/ssp/backup/config", payload, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error updating backup config", err.Error())
		return
	}

	mapSspBackupConfigToState(&result, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete removes the resource from state. The SSP API has no DELETE for backup config.
func (r *BackupConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
}

func (r *BackupConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var data BackupConfigResourceModel

	var result client.BackupConfig
	status, err := r.client.Get(ctx, "/ssp/backup/config", &result)
	if status == 204 {
		resp.Diagnostics.AddError("No backup config found", "No backup configuration is currently set.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error importing backup config", err.Error())
		return
	}

	mapSspBackupConfigToState(&result, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func buildSspBackupConfigPayload(m *BackupConfigResourceModel) *client.BackupConfig {
	cfg := &client.BackupConfig{
		ServerAddress:  m.ServerAddress.ValueString(),
		Protocol:       m.Protocol.ValueString(),
		Port:           int(m.Port.ValueInt64()),
		Username:       m.Username.ValueString(),
		SSHPublicKey:   m.SSHPublicKey.ValueString(),
		BackupLocation: m.BackupLocation.ValueString(),
		Password:       m.Password.ValueString(),
	}
	if !m.Passphrase.IsNull() && !m.Passphrase.IsUnknown() {
		cfg.Passphrase = m.Passphrase.ValueString()
	}
	return cfg
}

func mapSspBackupConfigToState(result *client.BackupConfig, m *BackupConfigResourceModel) {
	m.ID = types.StringValue("singleton")
	m.ServerAddress = types.StringValue(result.ServerAddress)
	m.Protocol = types.StringValue(result.Protocol)
	m.Port = types.Int64Value(int64(result.Port))
	m.Username = types.StringValue(result.Username)
	m.SSHPublicKey = types.StringValue(result.SSHPublicKey)
	m.BackupLocation = types.StringValue(strings.TrimRight(result.BackupLocation, "/"))
	// password and passphrase are write-only; preserve existing state values.
}
