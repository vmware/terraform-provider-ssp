package provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ resource.Resource = &ReadinessResource{}

func NewReadinessResource() resource.Resource {
	return &ReadinessResource{}
}

// ReadinessResource is a one-time gate that blocks until the SSP platform API is
// ready to accept authenticated requests. On Create it polls
// GET /ssp/cluster/monitor/platform/status until HTTP 200 is returned.
//
// On subsequent plan/apply cycles the resource is a no-op; it only re-runs
// when explicitly destroyed and recreated.
type ReadinessResource struct {
	client *client.Client
}

type ReadinessResourceModel struct {
	ID             types.String `tfsdk:"id"`
	TimeoutMinutes types.Int64  `tfsdk:"timeout_minutes"`
}

func (r *ReadinessResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_readiness"
}

func (r *ReadinessResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Waits for the SSP platform API to become ready before allowing dependent resources to proceed.

During ` + "`Create`" + `, polls ` + "`GET /ssp/cluster/monitor/platform/status`" + ` with the provider credentials
every 30 s until HTTP 200 is returned, confirming the cluster is operational.

On subsequent plan/apply cycles the resource is a no-op. Destroy and recreate it to
re-run the readiness check (e.g. after a platform redeployment).`,

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Always `done`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"timeout_minutes": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(20),
				MarkdownDescription: "Maximum number of minutes to wait for the SSP API to return HTTP 200. Defaults to `20`.",
			},
		},
	}
}

func (r *ReadinessResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ReadinessResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ReadinessResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout := time.Duration(data.TimeoutMinutes.ValueInt64()) * time.Minute
	if err := r.client.WaitForSSPAPIReady(ctx, timeout); err != nil {
		resp.Diagnostics.AddError("SSP API readiness check timed out", err.Error())
		return
	}

	data.ID = types.StringValue("done")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read is a no-op: the readiness gate is a one-time check and does not track
// ongoing state. The resource stays in state until explicitly destroyed.
func (r *ReadinessResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ReadinessResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update stores a new timeout value in state; no polling is re-run.
func (r *ReadinessResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ReadinessResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete is a no-op: there is nothing to remove on the remote side.
func (r *ReadinessResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}
