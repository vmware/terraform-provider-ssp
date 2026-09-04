package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ datasource.DataSource = &SitesDataSource{}

func NewSitesDataSource() datasource.DataSource {
	return &SitesDataSource{}
}

type SitesDataSource struct {
	client *client.Client
}

type SitesDataSourceModel struct {
	Sites []SiteSummaryModel `tfsdk:"sites"`
}

type SiteSummaryModel struct {
	ID               types.String `tfsdk:"id"`
	SiteType         types.String `tfsdk:"site_type"`
	SiteName         types.String `tfsdk:"site_name"`
	CurrentState     types.String `tfsdk:"current_state"`
	ConnectionStatus types.String `tfsdk:"connection_status"`
}

func (d *SitesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sites"
}

func (d *SitesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all sites (NSX Manager, AVI, SSP) currently connected to the SSP platform.\n\n" +
			"Corresponds to `GET /ssp/site-service/sites`.",
		Attributes: map[string]schema.Attribute{
			"sites": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of all onboarded sites.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                schema.StringAttribute{Computed: true, MarkdownDescription: "Site UUID."},
						"site_type":         schema.StringAttribute{Computed: true, MarkdownDescription: "Site type: NSX_MANAGER, AVI, or SSP."},
						"site_name":         schema.StringAttribute{Computed: true, MarkdownDescription: "Site display name."},
						"current_state":     schema.StringAttribute{Computed: true, MarkdownDescription: "Current readiness: READY, NOT_READY, INACTIVE, UNKNOWN."},
						"connection_status": schema.StringAttribute{Computed: true, MarkdownDescription: "Connection status: HEALTHY, UNHEALTHY, UNKNOWN."},
					},
				},
			},
		},
	}
}

func (d *SitesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SitesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data SitesDataSourceModel

	var list client.SiteList
	_, err := d.client.Get(ctx, "/ssp/site-service/sites", &list)
	if err != nil {
		resp.Diagnostics.AddError("Error listing sites", err.Error())
		return
	}

	data.Sites = make([]SiteSummaryModel, len(list.Sites))
	for i, s := range list.Sites {
		connStatus := ""
		if s.Status != nil {
			connStatus = s.Status.ConnectionStatus
		}
		data.Sites[i] = SiteSummaryModel{
			ID:               types.StringValue(s.ID),
			SiteType:         types.StringValue(s.SiteType),
			SiteName:         types.StringValue(s.SiteName),
			CurrentState:     types.StringValue(s.CurrentState),
			ConnectionStatus: types.StringValue(connStatus),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
