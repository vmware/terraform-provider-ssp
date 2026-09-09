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

var _ datasource.DataSource = &LicensesDataSource{}

func NewLicensesDataSource() datasource.DataSource {
	return &LicensesDataSource{}
}

type LicensesDataSource struct {
	client *client.Client
}

type LicensesDataSourceModel struct {
	Results []LicenseModel `tfsdk:"results"`
}

type LicenseModel struct {
	LicenseID          types.String `tfsdk:"license_id"`
	ProductDisplayName types.String `tfsdk:"product_display_name"`
	ProductFamily      types.String `tfsdk:"product_family"`
	Quantity           types.Int64  `tfsdk:"quantity"`
	UnitOfMeasure      types.String `tfsdk:"unit_of_measure"`
	Source             types.String `tfsdk:"source"`
	SkuCode            types.String `tfsdk:"sku_code"`
	ExpirationDate     types.Int64  `tfsdk:"expiration_date"`
}

func (d *LicensesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_licenses"
}

func (d *LicensesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all licenses registered with the SSP platform.\n\n" +
			"Corresponds to `GET /ssp/licensing-client/licenses`.",
		Attributes: map[string]schema.Attribute{
			"results": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of all active licenses.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"license_id":           schema.StringAttribute{Computed: true, MarkdownDescription: "License key / ID."},
						"product_display_name": schema.StringAttribute{Computed: true, MarkdownDescription: "Human-readable product name."},
						"product_family":       schema.StringAttribute{Computed: true, MarkdownDescription: "Product family (e.g. FIREWALL WITH ATP)."},
						"quantity":             schema.Int64Attribute{Computed: true, MarkdownDescription: "Licensed quantity."},
						"unit_of_measure":      schema.StringAttribute{Computed: true, MarkdownDescription: "Unit of measure for the quantity (e.g. Core)."},
						"source":               schema.StringAttribute{Computed: true, MarkdownDescription: "License source (e.g. VDLS)."},
						"sku_code":             schema.StringAttribute{Computed: true, MarkdownDescription: "SKU code for the license."},
						"expiration_date":      schema.Int64Attribute{Computed: true, MarkdownDescription: "License expiration as Unix epoch in milliseconds."},
					},
				},
			},
		},
	}
}

func (d *LicensesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *LicensesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data LicensesDataSourceModel

	var list client.LicenseList
	status, err := d.client.Get(ctx, "/ssp/licensing-client/licenses", &list)
	if err != nil {
		if status == 404 {
			data.Results = []LicenseModel{}
			resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
			return
		}
		resp.Diagnostics.AddError("Error listing licenses", err.Error())
		return
	}

	data.Results = make([]LicenseModel, len(list.Results))
	for i, l := range list.Results {
		data.Results[i] = LicenseModel{
			LicenseID:          types.StringValue(l.LicenseID),
			ProductDisplayName: types.StringValue(l.ProductDisplayName),
			ProductFamily:      types.StringValue(l.ProductFamily),
			Quantity:           types.Int64Value(int64(l.Quantity)),
			UnitOfMeasure:      types.StringValue(l.UnitOfMeasure),
			Source:             types.StringValue(l.Source),
			SkuCode:            types.StringValue(l.SkuCode),
			ExpirationDate:     types.Int64Value(l.ExpirationDate),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
