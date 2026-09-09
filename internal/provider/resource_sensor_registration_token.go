// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ resource.Resource = &SensorRegistrationTokenResource{}
var _ resource.ResourceWithImportState = &SensorRegistrationTokenResource{}

func NewSensorRegistrationTokenResource() resource.Resource {
	return &SensorRegistrationTokenResource{}
}

// SensorRegistrationTokenResource manages a sensor registration token.
type SensorRegistrationTokenResource struct {
	client *client.Client
}

// SensorRegistrationTokenResourceModel is the Terraform state model.
type SensorRegistrationTokenResourceModel struct {
	ID                   types.String `tfsdk:"id"`
	DisplayName          types.String `tfsdk:"display_name"`
	Description          types.String `tfsdk:"description"`
	Passphrase           types.String `tfsdk:"passphrase"`
	AllowedInstanceCount types.Int64  `tfsdk:"allowed_instance_count"`
	Status               types.String `tfsdk:"status"`
	Expiration           types.Int64  `tfsdk:"expiration"`
	UsageCount           types.Int64  `tfsdk:"usage_count"`
	NumberAvailable      types.Int64  `tfsdk:"number_available"`
	RegistrationManifest types.String `tfsdk:"registration_manifest"`
}

func (r *SensorRegistrationTokenResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sensor_registration_token"
}

func (r *SensorRegistrationTokenResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Creates a sensor registration token for onboarding NDR sensors to the SSP platform.\n\n" +
			"Tokens are valid for **30 minutes** and allow up to `allowed_instance_count` sensor registrations.\n" +
			"The `registration_manifest` attribute contains the value to supply to the sensor installer.\n\n" +
			"Corresponds to `POST/DELETE /sensors/registration-tokens`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Unique UUID of the token assigned by SSP.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"display_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name for the registration token.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional description for the token.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"passphrase": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "Passphrase to secure the registration token (minimum 15 characters). Write-only.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:          []validator.String{stringvalidator.LengthAtLeast(15)},
			},
			"allowed_instance_count": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(1),
				MarkdownDescription: "Maximum number of sensors that may register with this token. Defaults to `1`.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Token status: `AVAILABLE`, `EXHAUSTED`, or `EXPIRED`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"expiration": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Unix epoch timestamp (milliseconds) when the token expires.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"usage_count": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Number of sensors already registered with this token.",
			},
			"number_available": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Number of additional sensors that can still register with this token.",
			},
			"registration_manifest": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Opaque registration manifest to supply to the sensor installer. Sensitive.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *SensorRegistrationTokenResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SensorRegistrationTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SensorRegistrationTokenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := &client.SensorRegistrationToken{
		DisplayName:          data.DisplayName.ValueString(),
		Description:          data.Description.ValueString(),
		Passphrase:           data.Passphrase.ValueString(),
		AllowedInstanceCount: int(data.AllowedInstanceCount.ValueInt64()),
	}

	var result client.SensorRegistrationToken
	_, err := r.client.Post(ctx, "/sensors/registration-tokens", payload, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error creating sensor registration token", err.Error())
		return
	}

	mapTokenToState(&result, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SensorRegistrationTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SensorRegistrationTokenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result client.SensorRegistrationToken
	status, err := r.client.Get(ctx, "/sensors/registration-tokens/"+data.ID.ValueString(), &result)
	if status == 404 {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading sensor registration token", err.Error())
		return
	}

	mapTokenToState(&result, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update is not supported; all meaningful fields require replace.
func (r *SensorRegistrationTokenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update not supported",
		"sensor_registration_token resources cannot be updated in-place. Changes require replacement.")
}

func (r *SensorRegistrationTokenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SensorRegistrationTokenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	status, err := r.client.Delete(ctx, "/sensors/registration-tokens/"+data.ID.ValueString())
	if status == 404 {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error deleting sensor registration token", err.Error())
		return
	}
}

func (r *SensorRegistrationTokenResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var data SensorRegistrationTokenResourceModel
	data.ID = types.StringValue(req.ID)

	var result client.SensorRegistrationToken
	status, err := r.client.Get(ctx, "/sensors/registration-tokens/"+req.ID, &result)
	if status == 404 {
		resp.Diagnostics.AddError("Token not found", fmt.Sprintf("No sensor registration token with ID %s", req.ID))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error importing sensor registration token", err.Error())
		return
	}

	mapTokenToState(&result, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func mapTokenToState(result *client.SensorRegistrationToken, m *SensorRegistrationTokenResourceModel) {
	m.ID = types.StringValue(result.ID)
	m.DisplayName = types.StringValue(result.DisplayName)
	if result.Description != "" {
		m.Description = types.StringValue(result.Description)
	} else {
		m.Description = types.StringNull()
	}
	m.AllowedInstanceCount = types.Int64Value(int64(result.AllowedInstanceCount))
	m.Status = types.StringValue(result.Status)
	m.Expiration = types.Int64Value(result.Expiration)
	m.UsageCount = types.Int64Value(int64(result.UsageCount))
	m.NumberAvailable = types.Int64Value(int64(result.NumberAvailable))
	if result.RegistrationManifest != "" {
		m.RegistrationManifest = types.StringValue(result.RegistrationManifest)
	} else {
		m.RegistrationManifest = types.StringNull()
	}
	// passphrase is write-only; preserve existing state value.
}
