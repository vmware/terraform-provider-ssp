package provider

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"golang.org/x/net/proxy"

	"github.com/vmware/terraform-provider-ssp/internal/client/api_client"
	"github.com/vmware/terraform-provider-ssp/internal/client/depot_client"
	"github.com/vmware/terraform-provider-ssp/internal/client/iam_client"
	datasource_bundle "github.com/vmware/terraform-provider-ssp/internal/provider/datasource_bundle"
	datasource_platform "github.com/vmware/terraform-provider-ssp/internal/provider/datasource_platform"
	datasource_vsphere_provider "github.com/vmware/terraform-provider-ssp/internal/provider/datasource_vsphere_provider"
	resource_backup_config "github.com/vmware/terraform-provider-ssp/internal/provider/resource_backup_config"
	resource_bundle_local "github.com/vmware/terraform-provider-ssp/internal/provider/resource_bundle_local"
	resource_ldap_identity_source "github.com/vmware/terraform-provider-ssp/internal/provider/resource_ldap_identity_source"
	resource_platform "github.com/vmware/terraform-provider-ssp/internal/provider/resource_platform"
	resource_provider "github.com/vmware/terraform-provider-ssp/internal/provider/resource_provider"
	resource_recurring_backup_config "github.com/vmware/terraform-provider-ssp/internal/provider/resource_recurring_backup_config"
	resource_user_password "github.com/vmware/terraform-provider-ssp/internal/provider/resource_user_password"
)

var _ provider.Provider = &SspProvider{}
var _ provider.ProviderWithFunctions = &SspProvider{}

// SspProvider defines the provider implementation.
type SspProvider struct {
	version string
}

// SspProviderModel describes the provider data model.
type SspProviderModel struct {
	Host         types.String `tfsdk:"host"`
	Username     types.String `tfsdk:"username"`
	Password     types.String `tfsdk:"password"`
	Insecure     types.Bool   `tfsdk:"insecure"`
	SSPIHost     types.String `tfsdk:"sspi_host"`
	SSPIEndpoint types.String `tfsdk:"sspi_endpoint"`
	SSPIUsername types.String `tfsdk:"sspi_username"`
	SSPIPassword types.String `tfsdk:"sspi_password"`
	SSPIInsecure types.Bool   `tfsdk:"sspi_insecure"`
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
		MarkdownDescription: `The **SSP** (Security Services Platform) provider manages both the SSPI appliance
(installer, bootstrapping, cluster provisioning, scaling) and deployed SSP cluster instances (site onboarding,
vertical security features activation, backup/restore, telemetry, certificates).

Configure the provider with connection details for the SSPI appliance (for installer workflows) and/or the
deployed SSP cluster host (for Day-2 operational workflows). All sensitive values can be supplied via environment
variables to avoid storing secrets in configuration files.`,
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				MarkdownDescription: "Base HTTPS URL of the deployed SSP platform (e.g. `https://ssp.example.com`). " +
					"May also be set via the `SSP_HOST` environment variable.",
				Optional: true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "SSP enterprise admin username. " +
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
			"sspi_host": schema.StringAttribute{
				MarkdownDescription: "The base URL/host of the SSPI appliance. Can be specified with the `SSPI_HOST` environment variable.",
				Optional:            true,
			},
			"sspi_endpoint": schema.StringAttribute{
				MarkdownDescription: "The API endpoint of the SSPI appliance. Can be specified with the `SSPI_ENDPOINT` environment variable.",
				Optional:            true,
			},
			"sspi_username": schema.StringAttribute{
				MarkdownDescription: "The local administrator username for SSPI. Can be specified with the `SSPI_USERNAME` environment variable.",
				Optional:            true,
			},
			"sspi_password": schema.StringAttribute{
				MarkdownDescription: "The local administrator password for SSPI. Can be specified with the `SSPI_PASSWORD` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"sspi_insecure": schema.BoolAttribute{
				MarkdownDescription: "Allow insecure TLS connections to SSPI appliance. Can be specified with the `SSPI_INSECURE` environment variable. Defaults to false.",
				Optional:            true,
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

	// --- 1. SSP Runtime Configuration (Day-2) ---
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

	// --- 2. SSPI Appliance Configuration (Day-0/1) ---
	sspiEndpoint := os.Getenv("SSPI_HOST")
	if sspiEndpoint == "" {
		sspiEndpoint = os.Getenv("SSPI_ENDPOINT")
	}
	sspiUsername := os.Getenv("SSPI_USERNAME")
	sspiPassword := os.Getenv("SSPI_PASSWORD")

	if !data.SSPIHost.IsNull() && !data.SSPIHost.IsUnknown() {
		sspiEndpoint = data.SSPIHost.ValueString()
	} else if !data.SSPIEndpoint.IsNull() && !data.SSPIEndpoint.IsUnknown() {
		sspiEndpoint = data.SSPIEndpoint.ValueString()
	}
	if !data.SSPIUsername.IsNull() && !data.SSPIUsername.IsUnknown() {
		sspiUsername = data.SSPIUsername.ValueString()
	}
	if !data.SSPIPassword.IsNull() && !data.SSPIPassword.IsUnknown() {
		sspiPassword = data.SSPIPassword.ValueString()
	}

	sspiInsecure := false
	if !data.SSPIInsecure.IsNull() && !data.SSPIInsecure.IsUnknown() {
		sspiInsecure = data.SSPIInsecure.ValueBool()
	} else if os.Getenv("SSPI_INSECURE") == "true" {
		sspiInsecure = true
	}

	if sspiEndpoint != "" {
		if !strings.HasPrefix(sspiEndpoint, "http://") && !strings.HasPrefix(sspiEndpoint, "https://") {
			sspiEndpoint = "https://" + sspiEndpoint
		}
		sspiEndpoint = strings.TrimRight(sspiEndpoint, "/")

		transport := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: sspiInsecure,
			},
		}
		if strings.HasPrefix(sspiEndpoint, "http://127.0.0.1") || strings.HasPrefix(sspiEndpoint, "http://localhost") {
			transport.Proxy = nil
		} else if cd, ok := proxy.FromEnvironment().(proxy.ContextDialer); ok {
			transport.DialContext = cd.DialContext
		}

		httpClient := &http.Client{
			Timeout:   5 * time.Minute,
			Transport: transport,
		}

		authEditor := func(ctx context.Context, req *http.Request) error {
			req.SetBasicAuth(sspiUsername, sspiPassword)
			return nil
		}

		apiClient, err := api_client.NewClientWithResponses(
			sspiEndpoint,
			api_client.WithHTTPClient(httpClient),
			api_client.WithRequestEditorFn(authEditor),
		)
		if err != nil {
			resp.Diagnostics.AddError("Invalid SSPI Appliance Configuration", fmt.Sprintf("Unable to create SSPI API client: %s", err))
			return
		}
		clients.API = apiClient

		depotClient, err := depot_client.NewClientWithResponses(
			sspiEndpoint,
			depot_client.WithHTTPClient(httpClient),
			depot_client.WithRequestEditorFn(authEditor),
		)
		if err != nil {
			resp.Diagnostics.AddError("Invalid SSPI Appliance Configuration", fmt.Sprintf("Unable to create SSPI Depot client: %s", err))
			return
		}
		clients.Depot = depotClient

		iamClient, err := iam_client.NewClientWithResponses(
			sspiEndpoint,
			iam_client.WithHTTPClient(httpClient),
			iam_client.WithRequestEditorFn(authEditor),
		)
		if err != nil {
			resp.Diagnostics.AddError("Invalid SSPI Appliance Configuration", fmt.Sprintf("Unable to create SSPI IAM client: %s", err))
			return
		}
		clients.IAM = iamClient
	}

	resp.DataSourceData = clients
	resp.ResourceData = clients
}

func (p *SspProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		// SSPI Installer Resources
		resource_provider.NewProviderResource,
		resource_platform.NewPlatformResource,
		resource_bundle_local.NewBundleLocalResource,
		resource_ldap_identity_source.NewLdapIdentitySourceResource,
		resource_backup_config.NewBackupConfigResource,
		resource_recurring_backup_config.NewRecurringBackupConfigResource,
		resource_user_password.NewUserPasswordResource,

		// SSP Runtime Resources
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
		// apis/ssp_public_apis.yaml, apis/sspi/sspi-api.yaml, or sspi-iam.yaml expose a
		// certificate/CSR/CA-bundle import capability (the only /ssp/trust/* path is
		// trust-rollout-status, a read-only status endpoint), and the only
		// "/ssp/bundles*" paths belong to the unrelated security-content Mega Bundle
		// upload API (UploadMegaBundle/ListMegaBundle/GetMegaBundle/DeleteMegaBundle),
		// not a diagnostic/support-bundle capability. There is currently no real
		// backend for these four resource types, so no resource_*.go files exist for
		// them today -- add them only once the corresponding public API is published.
	}
}

func (p *SspProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		// SSPI Installer Data Sources
		datasource_vsphere_provider.NewVsphereProviderDataSource,
		datasource_platform.NewPlatformDataSource,
		datasource_bundle.NewBundleDataSource,

		// SSP Runtime Data Sources
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
