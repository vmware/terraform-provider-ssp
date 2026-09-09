// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ resource.Resource = &RecurringBackupConfigResource{}
var _ resource.ResourceWithImportState = &RecurringBackupConfigResource{}
var _ resource.ResourceWithValidateConfig = &RecurringBackupConfigResource{}

// objectAsOptions is the standard options for As calls on object types.
var objectAsOptions = basetypes.ObjectAsOptions{UnhandledNullAsEmpty: true, UnhandledUnknownAsEmpty: true}

func NewRecurringBackupConfigResource() resource.Resource {
	return &RecurringBackupConfigResource{}
}

// RecurringBackupConfigResource manages the automated backup schedule.
type RecurringBackupConfigResource struct {
	client *client.Client
}

// RecurringBackupConfigResourceModel is the Terraform state model.
type RecurringBackupConfigResourceModel struct {
	ID                     types.String `tfsdk:"id"`
	Enabled                types.Bool   `tfsdk:"enabled"`
	BackupType             types.String `tfsdk:"backup_type"`
	BackupScheduleType     types.String `tfsdk:"backup_schedule_type"`
	BackupScheduleWeekly   types.Object `tfsdk:"backup_schedule_weekly"`
	BackupScheduleInterval types.Object `tfsdk:"backup_schedule_interval"`
}

var weeklyScheduleAttrTypes = map[string]attr.Type{
	"days_of_week":  types.ListType{ElemType: types.StringType},
	"hour_of_day":   types.Int64Type,
	"minute_of_day": types.Int64Type,
}

var intervalScheduleAttrTypes = map[string]attr.Type{
	"hours_between_backups": types.Int64Type,
}

func (r *RecurringBackupConfigResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_recurring_backup_config"
}

func (r *RecurringBackupConfigResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the automated (recurring) backup schedule for the SSP platform.\n\n" +
			"This is a singleton resource — only one recurring backup policy can exist per SSP instance.\n" +
			"Use `terraform import ssp_recurring_backup_config.this singleton` to adopt an existing policy.\n\n" +
			"Set `backup_schedule_type` to `WEEKLY` and populate `backup_schedule_weekly`, or set it to\n" +
			"`INTERVAL` and populate `backup_schedule_interval`.\n\n" +
			"Corresponds to `GET/PUT /ssp/backup/recurring/config`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Always `singleton` for this resource.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: "When `true`, automated backups run on the configured schedule.",
			},
			"backup_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Backup scope. Must be `FULL_BACKUP`.",
			},
			"backup_schedule_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Schedule type: `WEEKLY` (calendar days/time) or `INTERVAL` (hours between runs).",
			},
			"backup_schedule_weekly": schema.SingleNestedAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Weekly schedule settings. Required when `backup_schedule_type = WEEKLY`.",
				PlanModifiers:       []planmodifier.Object{objectplanmodifier.UseStateForUnknown()},
				Attributes: map[string]schema.Attribute{
					"days_of_week": schema.ListAttribute{
						Required:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "Days of week when backup runs (e.g. `[\"MONDAY\", \"THURSDAY\"]`).",
					},
					"hour_of_day": schema.Int64Attribute{
						Optional:            true,
						Computed:            true,
						Default:             int64default.StaticInt64(0),
						MarkdownDescription: "Hour of day (0–23, UTC) when the backup starts. Defaults to `0`.",
					},
					"minute_of_day": schema.Int64Attribute{
						Optional:            true,
						Computed:            true,
						Default:             int64default.StaticInt64(0),
						MarkdownDescription: "Minute within the hour (0–59) when the backup starts. Defaults to `0`.",
					},
				},
			},
			"backup_schedule_interval": schema.SingleNestedAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Interval schedule settings. Required when `backup_schedule_type = INTERVAL`.",
				PlanModifiers:       []planmodifier.Object{objectplanmodifier.UseStateForUnknown()},
				Attributes: map[string]schema.Attribute{
					"hours_between_backups": schema.Int64Attribute{
						Optional:            true,
						Computed:            true,
						Default:             int64default.StaticInt64(168),
						MarkdownDescription: "Hours between consecutive automated backups (6–720). Defaults to `168` (weekly).",
					},
				},
			},
		},
	}
}

// ValidateConfig ensures backup_schedule_type and the matching nested schedule
// block are configured together. Without this, switching backup_schedule_type
// without also supplying the corresponding block would carry forward a stale
// nested object from prior state (via UseStateForUnknown) into a
// self-inconsistent PUT payload with no warning to the user.
func (r *RecurringBackupConfigResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data RecurringBackupConfigResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.BackupScheduleType.IsUnknown() || data.BackupScheduleType.IsNull() {
		return
	}

	switch data.BackupScheduleType.ValueString() {
	case "WEEKLY":
		if data.BackupScheduleWeekly.IsNull() {
			resp.Diagnostics.AddAttributeError(
				path.Root("backup_schedule_weekly"),
				"Missing Required Schedule Block",
				"backup_schedule_weekly must be set when backup_schedule_type is \"WEEKLY\".",
			)
		}
	case "INTERVAL":
		if data.BackupScheduleInterval.IsNull() {
			resp.Diagnostics.AddAttributeError(
				path.Root("backup_schedule_interval"),
				"Missing Required Schedule Block",
				"backup_schedule_interval must be set when backup_schedule_type is \"INTERVAL\".",
			)
		}
	}
}

func (r *RecurringBackupConfigResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RecurringBackupConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RecurringBackupConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload, d := buildRecurringConfigPayload(ctx, &data)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Fetch the current revision (if a recurring backup config already exists) so
	// this PUT isn't rejected by the backend's optimistic-locking check; a 204
	// means no config exists yet, so the zero-value Revision is correct as-is.
	var current client.RecurringBackupConfig
	status, err := r.client.Get(ctx, "/ssp/backup/recurring/config", &current)
	if status != 204 {
		if err != nil {
			resp.Diagnostics.AddError("Error reading current recurring backup config", err.Error())
			return
		}
		payload.Revision = current.Revision
	}

	var result client.RecurringBackupConfig
	_, err = r.client.Put(ctx, "/ssp/backup/recurring/config", payload, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error creating recurring backup config", err.Error())
		return
	}

	resp.Diagnostics.Append(mapRecurringConfigToState(ctx, &result, &data)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RecurringBackupConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RecurringBackupConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result client.RecurringBackupConfig
	status, err := r.client.Get(ctx, "/ssp/backup/recurring/config", &result)
	if status == 204 {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading recurring backup config", err.Error())
		return
	}

	resp.Diagnostics.Append(mapRecurringConfigToState(ctx, &result, &data)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RecurringBackupConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RecurringBackupConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload, d := buildRecurringConfigPayload(ctx, &data)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	var current client.RecurringBackupConfig
	if _, err := r.client.Get(ctx, "/ssp/backup/recurring/config", &current); err != nil {
		resp.Diagnostics.AddError("Error reading current recurring backup config", err.Error())
		return
	}
	payload.Revision = current.Revision

	var result client.RecurringBackupConfig
	_, err := r.client.Put(ctx, "/ssp/backup/recurring/config", payload, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error updating recurring backup config", err.Error())
		return
	}

	resp.Diagnostics.Append(mapRecurringConfigToState(ctx, &result, &data)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete removes the resource from state. No DELETE API for recurring backup config.
func (r *RecurringBackupConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
}

func (r *RecurringBackupConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var data RecurringBackupConfigResourceModel

	var result client.RecurringBackupConfig
	status, err := r.client.Get(ctx, "/ssp/backup/recurring/config", &result)
	if status == 204 {
		resp.Diagnostics.AddError("No recurring backup config found", "No recurring backup schedule is currently configured.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error importing recurring backup config", err.Error())
		return
	}

	resp.Diagnostics.Append(mapRecurringConfigToState(ctx, &result, &data)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func buildRecurringConfigPayload(ctx context.Context, m *RecurringBackupConfigResourceModel) (*client.RecurringBackupConfig, diag.Diagnostics) {
	var diags diag.Diagnostics
	cfg := &client.RecurringBackupConfig{
		Enabled:            m.Enabled.ValueBool(),
		BackupType:         m.BackupType.ValueString(),
		BackupScheduleType: m.BackupScheduleType.ValueString(),
	}

	if !m.BackupScheduleWeekly.IsNull() && !m.BackupScheduleWeekly.IsUnknown() {
		var weekly struct {
			DaysOfWeek  types.List  `tfsdk:"days_of_week"`
			HourOfDay   types.Int64 `tfsdk:"hour_of_day"`
			MinuteOfDay types.Int64 `tfsdk:"minute_of_day"`
		}
		diags.Append(m.BackupScheduleWeekly.As(ctx, &weekly, objectAsOptions)...)
		if diags.HasError() {
			return nil, diags
		}
		var days []string
		diags.Append(weekly.DaysOfWeek.ElementsAs(ctx, &days, false)...)
		if diags.HasError() {
			return nil, diags
		}
		cfg.BackupScheduleWeekly = &client.WeeklyBackupSchedule{
			DaysOfWeek:  days,
			HourOfDay:   int(weekly.HourOfDay.ValueInt64()),
			MinuteOfDay: int(weekly.MinuteOfDay.ValueInt64()),
		}
	}

	if !m.BackupScheduleInterval.IsNull() && !m.BackupScheduleInterval.IsUnknown() {
		var interval struct {
			HoursBetweenBackups types.Int64 `tfsdk:"hours_between_backups"`
		}
		diags.Append(m.BackupScheduleInterval.As(ctx, &interval, objectAsOptions)...)
		if diags.HasError() {
			return nil, diags
		}
		cfg.BackupScheduleInterval = &client.IntervalBackupSchedule{
			HoursBetweenBackups: int(interval.HoursBetweenBackups.ValueInt64()),
		}
	}

	return cfg, diags
}

func mapRecurringConfigToState(ctx context.Context, result *client.RecurringBackupConfig, m *RecurringBackupConfigResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	m.ID = types.StringValue("singleton")
	m.Enabled = types.BoolValue(result.Enabled)
	m.BackupType = types.StringValue(result.BackupType)
	m.BackupScheduleType = types.StringValue(result.BackupScheduleType)

	if result.BackupScheduleWeekly != nil {
		dayVals := make([]attr.Value, len(result.BackupScheduleWeekly.DaysOfWeek))
		for i, d := range result.BackupScheduleWeekly.DaysOfWeek {
			dayVals[i] = types.StringValue(d)
		}
		dayList, d := types.ListValue(types.StringType, dayVals)
		diags.Append(d...)

		weeklyObj, d := types.ObjectValue(weeklyScheduleAttrTypes, map[string]attr.Value{
			"days_of_week":  dayList,
			"hour_of_day":   types.Int64Value(int64(result.BackupScheduleWeekly.HourOfDay)),
			"minute_of_day": types.Int64Value(int64(result.BackupScheduleWeekly.MinuteOfDay)),
		})
		diags.Append(d...)
		if !diags.HasError() {
			m.BackupScheduleWeekly = weeklyObj
		}
	} else {
		m.BackupScheduleWeekly = types.ObjectNull(weeklyScheduleAttrTypes)
	}

	if result.BackupScheduleInterval != nil {
		intervalObj, d := types.ObjectValue(intervalScheduleAttrTypes, map[string]attr.Value{
			"hours_between_backups": types.Int64Value(int64(result.BackupScheduleInterval.HoursBetweenBackups)),
		})
		diags.Append(d...)
		if !diags.HasError() {
			m.BackupScheduleInterval = intervalObj
		}
	} else {
		m.BackupScheduleInterval = types.ObjectNull(intervalScheduleAttrTypes)
	}

	return diags
}
