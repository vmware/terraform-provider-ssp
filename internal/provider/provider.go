// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = &SspProvider{}
var _ provider.ProviderWithFunctions = &SspProvider{}

// SspProvider defines the provider implementation.
type SspProvider struct {
	version string
}

// SspProviderModel describes the provider data model.
type SspProviderModel struct {
	Host     types.String `tfsdk:"host"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
	Insecure types.Bool   `tfsdk:"insecure"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &SspProvider{
			version: version,
		}
	}
}

func (p *SspProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "ssp"
	resp.Version = p.version
}

func (p *SspProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `The **SSP** (Security Services Platform) provider manages deployed SSP cluster instances:
site onboarding, vertical security feature activation, backup/restore, telemetry, and certificates.

For Day-0/Day-1 SSPI appliance workflows (vCenter provider registration, package management, cluster
provisioning, scaling), see the companion ` + "`terraform-provider-sspi`" + ` provider.`,
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				MarkdownDescription: "Base HTTPS URL of the deployed SSP platform (e.g. `https://ssp.example.com`). " +
					"May also be set via the `SSP_HOST` environment variable.",
				Optional: true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "SSP username. Must hold the `enterprise_admin` role — nearly every " +
					"write operation across `ssp_*` resources requires it (alone, or paired with `auditor`); " +
					"an `auditor`-only or `security_op`-only account can read via `data.ssp_*` data sources " +
					"but will fail with an HTTP 403 on the first resource write. " +
					"May also be set via the `SSP_USERNAME` environment variable.",
				Optional: true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "SSP enterprise admin password. " +
					"May also be set via the `SSP_PASSWORD` environment variable.",
				Optional:  true,
				Sensitive: true,
			},
			"insecure": schema.BoolAttribute{
				MarkdownDescription: "When `true`, TLS certificate verification for the SSP cluster is skipped. " +
					"Defaults to `false`.",
				Optional: true,
			},
		},
	}
}

func (p *SspProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data SspProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clients := &ClientData{}

	runtimeHost := os.Getenv("SSP_HOST")
	if !data.Host.IsNull() && !data.Host.IsUnknown() {
		runtimeHost = data.Host.ValueString()
	}
	runtimeUsername := os.Getenv("SSP_USERNAME")
	if !data.Username.IsNull() && !data.Username.IsUnknown() {
		runtimeUsername = data.Username.ValueString()
	}
	runtimePassword := os.Getenv("SSP_PASSWORD")
	if !data.Password.IsNull() && !data.Password.IsUnknown() {
		runtimePassword = data.Password.ValueString()
	}
	runtimeInsecure := false
	if !data.Insecure.IsNull() && !data.Insecure.IsUnknown() {
		runtimeInsecure = data.Insecure.ValueBool()
	} else if os.Getenv("SSP_INSECURE") == "true" {
		runtimeInsecure = true
	}

	clients.RuntimeHost = runtimeHost
	clients.RuntimeUsername = runtimeUsername
	clients.RuntimePassword = runtimePassword
	clients.RuntimeInsecure = runtimeInsecure

	resp.DataSourceData = clients
	resp.ResourceData = clients
}

func (p *SspProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewReadinessResource,
		NewTelemetryConfigResource,
		NewBackupConfigResource,
		NewRecurringBackupConfigResource,
		NewBackupResource,
		NewRestoreResource,
		NewUpgradeResource,
		NewSiteResource,
		NewSensorRegistrationTokenResource,
		NewFeatureResource,
		NewMalwarePreventionConfigResource,
		NewCloudConnectorConfigResource,
		NewNdrConfigResource,
		// Certificate import, Ingress CSR generation, CA bundle import, and support
		// bundle generation/download are intentionally NOT implemented: none of
		// apis/ssp_public_apis.yaml exposes a certificate/CSR/CA-bundle import
		// capability (the only /ssp/trust/* path is trust-rollout-status, a
		// read-only status endpoint), and the only "/ssp/bundles*" paths belong to
		// the unrelated security-content Mega Bundle upload API
		// (UploadMegaBundle/ListMegaBundle/GetMegaBundle/DeleteMegaBundle), not a
		// diagnostic/support-bundle capability. There is currently no real backend
		// for these four resource types, so no resource_*.go files exist for them
		// today -- add them only once the corresponding public API is published.
	}
}

func (p *SspProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewPlatformStatusDataSource,
		NewFeatureHealthDataSource,
		NewSitesDataSource,
		NewSiteDataSource,
		NewLicensesDataSource,
		NewSensorsDataSource,
		NewBackupStatusDataSource,
		NewFeatureDataSource,
		NewUpgradeAvailableVersionsDataSource,
		NewUpgradeHistoryDataSource,
	}
}

func (p *SspProvider) Functions(ctx context.Context) []func() function.Function {
	return []func() function.Function{}
}
