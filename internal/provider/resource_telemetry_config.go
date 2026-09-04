package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ resource.Resource = &TelemetryConfigResource{}
var _ resource.ResourceWithImportState = &TelemetryConfigResource{}

func NewTelemetryConfigResource() resource.Resource {
	return &TelemetryConfigResource{}
}

// TelemetryConfigResource manages the singleton SSP telemetry (CEIP) configuration.
type TelemetryConfigResource struct {
	client *client.Client
}

// TelemetryConfigResourceModel is the Terraform state model.
type TelemetryConfigResourceModel struct {
	ID                           types.String `tfsdk:"id"`
	CeipAcceptance               types.Bool   `tfsdk:"ceip_acceptance"`
	TelemetryAgreementDisplayed  types.Bool   `tfsdk:"telemetry_agreement_displayed"`
	TelemetryCollectorInstanceID types.String `tfsdk:"telemetry_collector_instance_id"`
	// TelemetrySchedule is a types.Object (not a native struct pointer) because it is
	// Optional+Computed: a native struct pointer cannot represent the Unknown value the
	// framework assigns when telemetry_schedule is omitted from config on first apply.
	TelemetrySchedule types.Object `tfsdk:"telemetry_schedule"`
}

// TelemetryScheduleModel is the native Go representation of the nested telemetry_schedule object.
type TelemetryScheduleModel struct {
	FrequencyType types.String `tfsdk:"frequency_type"`
	HourOfDay     types.Int64  `tfsdk:"hour_of_day"`
	Minutes       types.Int64  `tfsdk:"minutes"`
	DayOfWeek     types.String `tfsdk:"day_of_week"`
	DayOfMonth    types.Int64  `tfsdk:"day_of_month"`
}

var telemetryScheduleAttrTypes = map[string]attr.Type{
	"frequency_type": types.StringType,
	"hour_of_day":    types.Int64Type,
	"minutes":        types.Int64Type,
	"day_of_week":    types.StringType,
	"day_of_month":   types.Int64Type,
}

func (r *TelemetryConfigResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_telemetry_config"
}

func (r *TelemetryConfigResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the SSP telemetry (CEIP) configuration.\n\n" +
			"This is a singleton resource — only one telemetry configuration exists per SSP platform.\n" +
			"Use `terraform import ssp_telemetry_config.this singleton` to adopt the existing configuration.\n\n" +
			"Corresponds to `GET/PUT /ssp/telemetry/config`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Always `singleton` for this resource.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"ceip_acceptance": schema.BoolAttribute{
				Required: true,
				MarkdownDescription: "When `true`, product usage data is collected and shared with " +
					"VMware by Broadcom as part of the Customer Experience Improvement Programme (CEIP).",
			},
			"telemetry_agreement_displayed": schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: "Whether the CEIP agreement dialog has been displayed to the user.",
			},
			"telemetry_collector_instance_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Read-only telemetry collector instance ID assigned by the platform.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"telemetry_schedule": schema.SingleNestedAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Schedule controlling when telemetry data is uploaded. " +
					"If omitted, the platform default (`WEEKLY_TELEMETRY_SCHEDULE` on `SUNDAY` at `02:00`) is used.",
				PlanModifiers: []planmodifier.Object{objectplanmodifier.UseStateForUnknown()},
				Attributes: map[string]schema.Attribute{
					"frequency_type": schema.StringAttribute{
						Optional: true,
						Computed: true,
						Default:  stringdefault.StaticString("WEEKLY_TELEMETRY_SCHEDULE"),
						MarkdownDescription: "Telemetry upload frequency. One of `DAILY_TELEMETRY_SCHEDULE` (requires `hour_of_day`/`minutes`), " +
							"`WEEKLY_TELEMETRY_SCHEDULE` (requires `day_of_week`/`hour_of_day`/`minutes`), or " +
							"`MONTHLY_TELEMETRY_SCHEDULE` (requires `day_of_month`/`hour_of_day`/`minutes`). Defaults to `WEEKLY_TELEMETRY_SCHEDULE`.",
						Validators: []validator.String{
							stringvalidator.OneOf("DAILY_TELEMETRY_SCHEDULE", "WEEKLY_TELEMETRY_SCHEDULE", "MONTHLY_TELEMETRY_SCHEDULE"),
						},
					},
					"hour_of_day": schema.Int64Attribute{
						Optional:            true,
						Computed:            true,
						Default:             int64default.StaticInt64(2),
						MarkdownDescription: "Hour of day (0-23) the telemetry upload runs. Defaults to `2`.",
						Validators: []validator.Int64{
							int64validator.Between(0, 23),
						},
					},
					"minutes": schema.Int64Attribute{
						Optional:            true,
						Computed:            true,
						Default:             int64default.StaticInt64(0),
						MarkdownDescription: "Minute of the hour (0-59) the telemetry upload runs. Defaults to `0`.",
						Validators: []validator.Int64{
							int64validator.Between(0, 59),
						},
					},
					"day_of_week": schema.StringAttribute{
						Optional: true,
						Computed: true,
						Default:  stringdefault.StaticString("SUNDAY"),
						MarkdownDescription: "Day of the week the telemetry upload runs. Only applicable when `frequency_type` is " +
							"`WEEKLY_TELEMETRY_SCHEDULE`. Defaults to `SUNDAY`.",
						Validators: []validator.String{
							stringvalidator.OneOf("SUNDAY", "MONDAY", "TUESDAY", "WEDNESDAY", "THURSDAY", "FRIDAY", "SATURDAY"),
						},
					},
					"day_of_month": schema.Int64Attribute{
						Optional: true,
						Computed: true,
						Default:  int64default.StaticInt64(1),
						MarkdownDescription: "Day of the month (1-31) the telemetry upload runs. Only applicable when `frequency_type` is " +
							"`MONTHLY_TELEMETRY_SCHEDULE`. Defaults to `1`.",
						Validators: []validator.Int64{
							int64validator.Between(1, 31),
						},
					},
				},
			},
		},
	}
}

func (r *TelemetryConfigResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TelemetryConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TelemetryConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var current client.TelemetryConfig
	if _, err := r.client.Get(ctx, "/ssp/telemetry/config", &current); err != nil {
		resp.Diagnostics.AddError("Error reading current telemetry config", err.Error())
		return
	}

	schedule, diags := telemetryScheduleToAPI(ctx, data.TelemetrySchedule)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := &client.TelemetryConfig{
		Revision:                    current.Revision,
		CeipAcceptance:              data.CeipAcceptance.ValueBool(),
		TelemetryAgreementDisplayed: data.TelemetryAgreementDisplayed.ValueBool(),
		TelemetrySchedule:           schedule,
	}

	var result client.TelemetryConfig
	_, err := r.client.Put(ctx, "/ssp/telemetry/config", payload, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error creating telemetry config", err.Error())
		return
	}

	if result.TelemetryCollectorInstanceID == "" {
		_, _ = r.client.Get(ctx, "/ssp/telemetry/config", &result)
	}

	resp.Diagnostics.Append(mapTelemetryToState(ctx, &result, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TelemetryConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TelemetryConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result client.TelemetryConfig
	_, err := r.client.Get(ctx, "/ssp/telemetry/config", &result)
	if err != nil {
		resp.Diagnostics.AddError("Error reading telemetry config", err.Error())
		return
	}

	resp.Diagnostics.Append(mapTelemetryToState(ctx, &result, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TelemetryConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data TelemetryConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var current client.TelemetryConfig
	if _, err := r.client.Get(ctx, "/ssp/telemetry/config", &current); err != nil {
		resp.Diagnostics.AddError("Error reading current telemetry config", err.Error())
		return
	}

	schedule, diags := telemetryScheduleToAPI(ctx, data.TelemetrySchedule)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := &client.TelemetryConfig{
		Revision:                    current.Revision,
		CeipAcceptance:              data.CeipAcceptance.ValueBool(),
		TelemetryAgreementDisplayed: data.TelemetryAgreementDisplayed.ValueBool(),
		TelemetrySchedule:           schedule,
	}

	var result client.TelemetryConfig
	_, err := r.client.Put(ctx, "/ssp/telemetry/config", payload, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error updating telemetry config", err.Error())
		return
	}

	if result.TelemetryCollectorInstanceID == "" {
		_, _ = r.client.Get(ctx, "/ssp/telemetry/config", &result)
	}

	resp.Diagnostics.Append(mapTelemetryToState(ctx, &result, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete removes the resource from Terraform state. The SSP API does not support
// deleting the telemetry configuration; it is a persistent platform setting.
func (r *TelemetryConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
}

func (r *TelemetryConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var data TelemetryConfigResourceModel

	var result client.TelemetryConfig
	_, err := r.client.Get(ctx, "/ssp/telemetry/config", &result)
	if err != nil {
		resp.Diagnostics.AddError("Error importing telemetry config", err.Error())
		return
	}

	resp.Diagnostics.Append(mapTelemetryToState(ctx, &result, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func mapTelemetryToState(ctx context.Context, result *client.TelemetryConfig, m *TelemetryConfigResourceModel) diag.Diagnostics {
	m.ID = types.StringValue("singleton")
	m.CeipAcceptance = types.BoolValue(result.CeipAcceptance)
	m.TelemetryAgreementDisplayed = types.BoolValue(result.TelemetryAgreementDisplayed)
	m.TelemetryCollectorInstanceID = types.StringValue(result.TelemetryCollectorInstanceID)

	schedule, diags := telemetryScheduleFromAPI(ctx, result.TelemetrySchedule)
	m.TelemetrySchedule = schedule
	return diags
}

// telemetryScheduleToAPI converts the Terraform nested object to the API payload shape.
func telemetryScheduleToAPI(ctx context.Context, o types.Object) (*client.TelemetrySchedule, diag.Diagnostics) {
	if o.IsNull() || o.IsUnknown() {
		return nil, nil
	}
	var m TelemetryScheduleModel
	diags := o.As(ctx, &m, objectAsOptions)
	if diags.HasError() {
		return nil, diags
	}
	return &client.TelemetrySchedule{
		FrequencyType: m.FrequencyType.ValueString(),
		HourOfDay:     int(m.HourOfDay.ValueInt64()),
		Minutes:       int(m.Minutes.ValueInt64()),
		DayOfWeek:     m.DayOfWeek.ValueString(),
		DayOfMonth:    int(m.DayOfMonth.ValueInt64()),
	}, nil
}

// telemetryScheduleFromAPI converts the API response shape to the Terraform nested object.
func telemetryScheduleFromAPI(ctx context.Context, s *client.TelemetrySchedule) (types.Object, diag.Diagnostics) {
	if s == nil {
		return types.ObjectNull(telemetryScheduleAttrTypes), nil
	}
	return types.ObjectValueFrom(ctx, telemetryScheduleAttrTypes, &TelemetryScheduleModel{
		FrequencyType: types.StringValue(s.FrequencyType),
		HourOfDay:     types.Int64Value(int64(s.HourOfDay)),
		Minutes:       types.Int64Value(int64(s.Minutes)),
		DayOfWeek:     types.StringValue(s.DayOfWeek),
		DayOfMonth:    types.Int64Value(int64(s.DayOfMonth)),
	})
}
