// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ datasource.DataSource = &FeatureDataSource{}

func NewFeatureDataSource() datasource.DataSource {
	return &FeatureDataSource{}
}

// FeatureDataSource reads the current deployment status of a single SSP feature.
type FeatureDataSource struct {
	client *client.Client
}

// FeatureDataSourceModel is the Terraform state model.
type FeatureDataSourceModel struct {
	Feature          types.String `tfsdk:"feature"`
	OverallStatus    types.String `tfsdk:"overall_status"`
	OverallProgress  types.Int64  `tfsdk:"overall_progress"`
	PrecheckStatus   types.String `tfsdk:"precheck_status"`
	DeploymentStatus types.String `tfsdk:"deployment_status"`
}

func (d *FeatureDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_feature"
}

func (d *FeatureDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the current deployment status of a vDefend feature on the SSP platform.\n\n" +
			"Corresponds to `GET /ssp/lcm/features/{feature}/status`.",
		Attributes: map[string]schema.Attribute{
			"feature": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the feature to query (case-sensitive, e.g. `INTELLIGENCE`).",
			},
			"overall_status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current overall deployment status (e.g. `DEPLOYMENT_SUCCESSFUL`, `NOT_DEPLOYED`).",
			},
			"overall_progress": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Overall deployment progress as a percentage (0–100).",
			},
			"precheck_status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Overall status of the most recent pre-check run.",
			},
			"deployment_status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Detailed deployment status from the deployment results object.",
			},
		},
	}
}

func (d *FeatureDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, err := GetClientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected provider data type", err.Error())
		return
	}
	d.client = c
}

func (d *FeatureDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data FeatureDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	feature := data.Feature.ValueString()
	var status client.FeatureDeploymentStatus
	_, err := d.client.Get(ctx, "/ssp/lcm/features/"+feature+"/status", &status)
	if err != nil {
		resp.Diagnostics.AddError("Error reading feature status for "+feature, err.Error())
		return
	}

	data.OverallStatus = types.StringValue(status.OverallStatus)
	data.OverallProgress = types.Int64Value(int64(status.OverallProgress))
	data.PrecheckStatus = types.StringValue(status.PrecheckResults.OverallStatus)
	data.DeploymentStatus = types.StringValue(status.DeploymentResults.DeploymentStatus)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
