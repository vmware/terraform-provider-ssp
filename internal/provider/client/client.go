// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

// Package client provides an HTTP client for the SSP REST API.
package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

// defaultPollInterval and defaultPollTimeout are vars, not consts, so unit
// tests in this package can temporarily shrink them (save/restore) to
// exercise the multi-iteration polling loop and deadline logic below in
// milliseconds instead of the real 15s/90min production values.
var (
	defaultPollInterval = 15 * time.Second
	defaultPollTimeout  = 90 * time.Minute

	// sspAPIReadyPollInterval is WaitForSSPAPIReady's own retry interval
	// (30s in production), kept separate from defaultPollInterval since its
	// caller (ssp_readiness) supplies its own timeout rather than using
	// defaultPollTimeout. Also a var, not a const, so unit tests can shrink it.
	sspAPIReadyPollInterval = 30 * time.Second
)

// Client is an authenticated HTTP client for the SSP API.
type Client struct {
	host       string
	username   string
	password   string
	httpClient *http.Client
}

// NewClient creates a new SSP API client.
func NewClient(host, username, password string, insecure bool) *Client {
	transport := &http.Transport{
		Proxy:           http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure, MinVersion: tls.VersionTLS12}, //nolint:gosec
	}
	if strings.HasPrefix(host, "http://127.0.0.1") || strings.HasPrefix(host, "http://localhost") {
		transport.Proxy = nil
	} else if cd, ok := proxy.FromEnvironment().(proxy.ContextDialer); ok {
		transport.DialContext = cd.DialContext
	}
	return &Client{
		host:     host,
		username: username,
		password: password,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   60 * time.Second,
		},
	}
}

// ── Shared API response types ──────────────────────────────────────────────────

// AsyncApiResponse is returned by async (202) API calls.
type AsyncApiResponse struct {
	ID        string `json:"id"`
	StatusURL string `json:"status_url"`
}

// ── Telemetry ──────────────────────────────────────────────────────────────────

// TelemetryConfig represents the SSP CEIP / telemetry settings.
type TelemetryConfig struct {
	ID                           string             `json:"id,omitempty"`
	Revision                     int                `json:"_revision"`
	CeipAcceptance               bool               `json:"ceip_acceptance"`
	TelemetryAgreementDisplayed  bool               `json:"telemetry_agreement_displayed"`
	TelemetryCollectorInstanceID string             `json:"telemetry_collector_instance_id,omitempty"`
	TelemetrySchedule            *TelemetrySchedule `json:"telemetry_schedule,omitempty"`
}

// TelemetrySchedule defines when telemetry data is uploaded.
type TelemetrySchedule struct {
	FrequencyType string `json:"frequency_type,omitempty"`
	HourOfDay     int    `json:"hour_of_day,omitempty"`
	Minutes       int    `json:"minutes,omitempty"`
	DayOfWeek     string `json:"day_of_week,omitempty"`
	DayOfMonth    int    `json:"day_of_month,omitempty"`
}

// ── Backup ─────────────────────────────────────────────────────────────────────

// BackupConfig represents the SFTP remote backup target.
type BackupConfig struct {
	ID             string `json:"id,omitempty"`
	Revision       int    `json:"_revision"`
	ServerAddress  string `json:"server_address"`
	Protocol       string `json:"protocol"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	SSHPublicKey   string `json:"ssh_public_key"`
	BackupLocation string `json:"backup_location"`
	Password       string `json:"password,omitempty"`
	Passphrase     string `json:"passphrase,omitempty"`
}

// RecurringBackupConfig represents the automated backup schedule.
type RecurringBackupConfig struct {
	ID                     string                  `json:"id,omitempty"`
	Revision               int                     `json:"_revision"`
	Enabled                bool                    `json:"enabled"`
	BackupType             string                  `json:"backup_type"`
	BackupScheduleType     string                  `json:"backup_schedule_type"`
	BackupScheduleWeekly   *WeeklyBackupSchedule   `json:"backup_schedule_weekly,omitempty"`
	BackupScheduleInterval *IntervalBackupSchedule `json:"backup_schedule_interval,omitempty"`
}

// WeeklyBackupSchedule defines days/times for WEEKLY schedules.
type WeeklyBackupSchedule struct {
	DaysOfWeek  []string `json:"days_of_week"`
	HourOfDay   int      `json:"hour_of_day"`
	MinuteOfDay int      `json:"minute_of_day"`
}

// IntervalBackupSchedule defines hours between runs for INTERVAL schedules.
type IntervalBackupSchedule struct {
	HoursBetweenBackups int `json:"hours_between_backups"`
}

// BackupStatusList is the paginated response for GET /ssp/backup/status.
type BackupStatusList struct {
	Results          []BackupStatus `json:"results"`
	TotalResultCount int            `json:"total_result_count"`
}

// BackupStatus summarises a single backup job.
type BackupStatus struct {
	ID                string   `json:"id"`
	Status            string   `json:"status"`
	BackupType        string   `json:"backup_type"`
	BackupTriggerType string   `json:"backup_trigger_type,omitempty"`
	BackupStartTime   int64    `json:"backup_start_time,omitempty"`
	BackupEndTime     int64    `json:"backup_end_time,omitempty"`
	Progress          int      `json:"progress,omitempty"`
	ProgressMessage   string   `json:"progress_message,omitempty"`
	Version           string   `json:"version,omitempty"`
	FileSize          int64    `json:"file_size,omitempty"`
	ErrorMessages     []string `json:"error_messages,omitempty"`
	NoOfEntities      int      `json:"no_of_entities,omitempty"`
}

// BackupRequest represents a request to trigger an on-demand backup.
type BackupRequest struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	BackupType  string `json:"backup_type"`
	Action      string `json:"action"`
}

// RestoreRequest represents a request to trigger a restore operation.
type RestoreRequest struct {
	ID           string `json:"id,omitempty"`
	Name         string `json:"name,omitempty"`
	Description  string `json:"description,omitempty"`
	BackupID     string `json:"backup_id"`
	ForceRestore bool   `json:"force_restore,omitempty"`
	Action       string `json:"action"`
}

// RestoreStatus represents the status of a restore operation.
type RestoreStatus struct {
	ID              string   `json:"id"`
	BackupID        string   `json:"backup_id,omitempty"`
	Status          string   `json:"status"`
	Progress        int      `json:"progress,omitempty"`
	ProgressMessage string   `json:"progress_message,omitempty"`
	ErrorMessages   []string `json:"error_messages,omitempty"`
	NoOfEntities    int      `json:"no_of_entities,omitempty"`
}

// ── Sites ──────────────────────────────────────────────────────────────────────

// Site is the top-level site object sent to POST/PUT and received from GET.
type Site struct {
	ID                 string          `json:"id,omitempty"`
	Revision           int             `json:"_revision"`
	SiteType           string          `json:"site_type"`
	SiteName           string          `json:"site_name,omitempty"`
	DesiredState       string          `json:"desired_state,omitempty"`
	CurrentState       string          `json:"current_state,omitempty"`
	SiteConnectionInfo *SiteConnection `json:"site_connection_info,omitempty"`
	// Status fields (NSX specific)
	Status *NsxSiteStatus `json:"status,omitempty"`
}

// SiteConnection holds the connection parameters for a site.
// Supports DYNAMIC (hostname) and STATIC (host_addresses) modes.
type SiteConnection struct {
	ConnectionType string   `json:"connection_type"`
	Hostname       string   `json:"hostname,omitempty"`
	HostAddresses  []string `json:"host_addresses,omitempty"`
	Username       string   `json:"username,omitempty"`
	Password       string   `json:"password,omitempty"`
	Certificate    string   `json:"certificate,omitempty"`
}

// UserCredentials is required by the DELETE /ssp/sites/{id} body.
type UserCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// NsxSiteStatus holds the read-only runtime status of an NSX Manager site.
type NsxSiteStatus struct {
	ConnectionStatus string `json:"connection_status,omitempty"`
	ClusterStatus    string `json:"cluster_status,omitempty"`
	NsxClusterID     string `json:"nsx_cluster_id,omitempty"`
	NsxVersion       string `json:"nsx_version,omitempty"`
}

// SiteList is the response from GET /ssp/sites.
type SiteList struct {
	Sites            []Site `json:"sites"`
	TotalResultCount int    `json:"total_result_count"`
}

// ── Sensors ────────────────────────────────────────────────────────────────────

// SensorRegistrationToken is the request/response for sensor registration tokens.
type SensorRegistrationToken struct {
	ID                   string `json:"id,omitempty"`
	DisplayName          string `json:"display_name"`
	Description          string `json:"description,omitempty"`
	Passphrase           string `json:"passphrase,omitempty"`
	AllowedInstanceCount int    `json:"allowed_instance_count,omitempty"`
	Status               string `json:"status,omitempty"`
	Expiration           int64  `json:"expiration,omitempty"`
	UsageCount           int    `json:"usage_count,omitempty"`
	NumberAvailable      int    `json:"number_available,omitempty"`
	RegistrationManifest string `json:"registration_manifest,omitempty"`
}

// SensorRegistrationTokenList is the paginated list of tokens.
type SensorRegistrationTokenList struct {
	Results          []SensorRegistrationToken `json:"results"`
	TotalResultCount int                       `json:"total_result_count"`
}

// Sensor represents a registered sensor appliance.
type Sensor struct {
	ID          string `json:"id,omitempty"`
	DisplayName string `json:"display_name"`
	Description string `json:"description,omitempty"`
	TokenID     string `json:"token_id,omitempty"`
	Certificate string `json:"certificate,omitempty"`
	PemEncoded  string `json:"pem_encoded,omitempty"`
	Algorithm   string `json:"algorithm,omitempty"`
	KeySize     int    `json:"key_size,omitempty"`
}

// SensorList is the paginated response for GET /sensors/appliances.
type SensorList struct {
	Results          []Sensor `json:"results"`
	TotalResultCount int      `json:"total_result_count"`
}

// ── Upgrade ────────────────────────────────────────────────────────────────────

// UpgradeRequest represents the body for POST /ssp/upgrade.
type UpgradeRequest struct {
	TargetVersion string `json:"target_version"`
	PreChecks     bool   `json:"pre_checks,omitempty"`
}

// UpgradeStatus holds the status of an ongoing or completed upgrade.
//
// GET /ssp/upgrade/status (SspUpgradeStatus) reports the live status under
// "overall_status" (OverallStatus below); GET /ssp/upgrade/history reports
// each historical attempt under "status" (Status below) — the two endpoints
// use different field names for a conceptually similar value, so both are
// kept here rather than merging into one field.
type UpgradeStatus struct {
	ID             string            `json:"id,omitempty"`
	Status         string            `json:"status,omitempty"`
	OverallStatus  string            `json:"overall_status,omitempty"`
	Progress       int               `json:"progress,omitempty"`
	CurrentStep    string            `json:"current_step,omitempty"`
	CurrentVersion string            `json:"current_version,omitempty"`
	TargetVersion  string            `json:"target_version,omitempty"`
	UpgradeSteps   []UpgradeStepInfo `json:"upgrade_steps,omitempty"`
	ErrorMessages  []string          `json:"error_messages,omitempty"`
}

// UpgradeStepInfo is a single entry of SspUpgradeStatus.upgrade_steps.
type UpgradeStepInfo struct {
	ID          string `json:"id,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Status      string `json:"status,omitempty"`
}

// CurrentStepName returns the display name of the first in-progress upgrade
// step, or the last step in the list if none is currently running.
func (s *UpgradeStatus) CurrentStepName() string {
	for _, step := range s.UpgradeSteps {
		if step.Status == "IN_PROGRESS" {
			return step.DisplayName
		}
	}
	if n := len(s.UpgradeSteps); n > 0 {
		return s.UpgradeSteps[n-1].DisplayName
	}
	return ""
}

// StepProgressPercent returns the percentage of upgrade_steps in a terminal
// success state (SUCCESS or SUCCESS_WITH_WARNINGS).
func (s *UpgradeStatus) StepProgressPercent() int {
	if len(s.UpgradeSteps) == 0 {
		return 0
	}
	done := 0
	for _, step := range s.UpgradeSteps {
		if step.Status == "SUCCESS" || step.Status == "SUCCESS_WITH_WARNINGS" || step.Status == "SKIPPED" {
			done++
		}
	}
	return done * 100 / len(s.UpgradeSteps)
}

// AvailableVersionList is the response from GET /ssp/upgrade/available-versions.
type AvailableVersionList struct {
	Versions []string `json:"versions"`
}

// UpgradeHistoryList is the response from GET /ssp/upgrade/history.
type UpgradeHistoryList struct {
	Results []UpgradeStatus `json:"results"`
}

// ClusterStatus is the response from GET /ssp/cluster/monitor/platform/status.
type ClusterStatus struct {
	ClusterID          string       `json:"cluster_id,omitempty"`
	ClusterName        string       `json:"cluster_name,omitempty"`
	ProductVersion     string       `json:"product_version,omitempty"`
	NodeCount          int          `json:"node_count,omitempty"`
	FormFactor         string       `json:"form_factor,omitempty"`
	Health             string       `json:"health,omitempty"`
	MessageBusEndpoint string       `json:"message_bus_endpoint,omitempty"`
	K8sVersion         string       `json:"k8s_version,omitempty"`
	IngressURL         string       `json:"ingress_url,omitempty"`
	NetworkDataFlow    *NetworkData `json:"network_data_flow,omitempty"`
}

// NetworkData holds network transmit/receive/total rate statistics.
type NetworkData struct {
	Transmit float64 `json:"transmit,omitempty"`
	Receive  float64 `json:"receive,omitempty"`
	Total    float64 `json:"total,omitempty"`
}

// FeatureHealthResponse is the response from GET /ssp/cluster/monitor/feature/health.
type FeatureHealthResponse struct {
	OverallStatus string          `json:"overall_status,omitempty"`
	Features      []FeatureHealth `json:"features,omitempty"`
}

// FeatureHealth holds health for one platform feature.
type FeatureHealth struct {
	Name   string `json:"name,omitempty"`
	Status string `json:"status,omitempty"`
}

// ── Licenses ──────────────────────────────────────────────────────────────────

// LicenseList is the response from GET /ssp/licenses.
type LicenseList struct {
	Results          []License `json:"results"`
	TotalResultCount int       `json:"total_result_count"`
}

// License holds the details of one SSP license.
type License struct {
	LicenseID                 string `json:"license_id,omitempty"`
	LicenseType               string `json:"license_type,omitempty"`
	ProductDisplayName        string `json:"product_display_name,omitempty"`
	ProductFamily             string `json:"product_family,omitempty"`
	Quantity                  int    `json:"quantity,omitempty"`
	UnitOfMeasure             string `json:"unit_of_measure,omitempty"`
	Source                    string `json:"source,omitempty"`
	SourceID                  string `json:"source_id,omitempty"`
	SkuCode                   string `json:"sku_code,omitempty"`
	ExpirationDate            int64  `json:"expiration_date,omitempty"`
	ExpiryDateWithGracePeriod int64  `json:"expiry_date_with_grace_period,omitempty"`
}

// NdrConfiguration is used for GET and PUT /ssp/lcm/ndr/config.
type NdrConfiguration struct {
	Revision    int  `json:"_revision"`
	DataSharing bool `json:"data_sharing"`
}

// CloudConnectorConfiguration is used for GET and PUT
// /ssp/lcm/cloud-connector/config.
type CloudConnectorConfiguration struct {
	Revision     int    `json:"_revision"`
	FQDN         string `json:"fqdn"`
	Region       string `json:"region"`
	RegionName   string `json:"region_name"`
	Configurable *bool  `json:"configurable,omitempty"` // read-only
}

// CloudConnectorRegion describes a single available cloud region.
type CloudConnectorRegion struct {
	FQDN       string `json:"fqdn"`
	Region     string `json:"region"`
	RegionName string `json:"region_name"`
}

// AvailableCloudConnectorRegions is the response from
// GET /ssp/lcm/cloud-connector/regions.
type AvailableCloudConnectorRegions struct {
	CloudRegions []CloudConnectorRegion `json:"cloud_regions"`
}

// CloudConnectorConfigurationStatus is returned by
// GET /ssp/lcm/cloud-connector/config/status.
type CloudConnectorConfigurationStatus struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// ── Malware Prevention configuration ─────────────────────────────────────────

// MalwarePreventionConfiguration is used for GET and PUT
// /ssp/lcm/malware-prevention/config.
type MalwarePreventionConfiguration struct {
	Revision               int                                          `json:"_revision"`
	AnalysisMode           string                                       `json:"analysis_mode"`
	CloudFileSharingParams *MalwarePreventionCloudFileSharingParameters `json:"cloud_file_sharing_params,omitempty"`
	DataRetentionParams    *MalwarePreventionDataRetentionParameters    `json:"data_retention_params,omitempty"`
}

// MalwarePreventionCloudFileSharingParameters controls cloud data-sharing for
// the CLOUD analysis mode.
type MalwarePreventionCloudFileSharingParameters struct {
	FileMetadataSharingMode bool                                 `json:"file_metadata_sharing_mode"`
	FileSharingMode         bool                                 `json:"file_sharing_mode"`
	FileTypeCategories      *MalwarePreventionFileTypeCategories `json:"file_type_categories,omitempty"`
}

// MalwarePreventionFileTypeCategories controls which file types are analysed.
type MalwarePreventionFileTypeCategories struct {
	Archive    bool `json:"archive"`
	Document   bool `json:"document"`
	Executable bool `json:"executable"`
	Java       bool `json:"java"`
	Media      bool `json:"media"`
	Script     bool `json:"script"`
}

// MalwarePreventionDataRetentionParameters controls retention for the ON_PREM
// analysis mode.
type MalwarePreventionDataRetentionParameters struct {
	FileRetentionDuration   int `json:"file_retention_duration"`
	ReportRetentionDuration int `json:"report_retention_duration"`
}

// MalwarePreventionConfigurationStatus is returned by
// GET /ssp/lcm/malware-prevention/config/status.
type MalwarePreventionConfigurationStatus struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// ── Feature LCM ───────────────────────────────────────────────────────────────

// FeatureDeployment is used for GET and PUT /ssp/lcm/features/{feature}.
type FeatureDeployment struct {
	Revision int    `json:"_revision"`
	Action   string `json:"action"`
}

// FeatureDeploymentStatus is returned by GET /ssp/lcm/features/{feature}/status.
type FeatureDeploymentStatus struct {
	Feature           string            `json:"feature"`
	Action            string            `json:"action"`
	OverallStatus     string            `json:"overall_status"`
	OverallProgress   int               `json:"overall_progress"`
	PrecheckResults   PrecheckResults   `json:"precheck_results"`
	DeploymentResults DeploymentResults `json:"deployment_results"`
}

// PrecheckResults holds the pre-check execution summary.
type PrecheckResults struct {
	Revision      int            `json:"_revision"`
	SystemStatus  string         `json:"system_status,omitempty"`
	OverallStatus string         `json:"overall_status"`
	Prechecks     []PrecheckItem `json:"prechecks"`
	Progress      int            `json:"progress"`
	TotalChecks   int            `json:"total_checks"`
	PassedChecks  int            `json:"passed_checks"`
	FailedChecks  int            `json:"failed_checks"`
}

// PrecheckItem holds the result of a single pre-check.
type PrecheckItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Feature     string `json:"feature"`
	Status      string `json:"status"`
	Reason      string `json:"reason"`
}

// DeploymentResults holds the deployment execution results.
type DeploymentResults struct {
	Revision           int                      `json:"_revision"`
	DeploymentStatus   string                   `json:"deployment_status"`
	Reason             string                   `json:"reason"`
	DeploymentProgress DeploymentProgress       `json:"deployment_progress"`
	DeploymentDetails  []DeploymentActionStatus `json:"deployment_details"`
}

// DeploymentProgress holds deployment progress details.
type DeploymentProgress struct {
	Percentage int    `json:"percentage"`
	Message    string `json:"message"`
}

// DeploymentActionStatus holds the status of a single deployment action step.
type DeploymentActionStatus struct {
	Action string `json:"action"`
	Status string `json:"status"`
}

// WaitForFeaturePrechecks polls GET /ssp/lcm/features/{feature}/status until
// the precheck overall_status reaches a terminal state.
func (c *Client) WaitForFeaturePrechecks(ctx context.Context, feature string) (*FeatureDeploymentStatus, error) {
	deadline := time.Now().Add(defaultPollTimeout)
	var lastErr error
	for {
		if time.Now().After(deadline) {
			if lastErr != nil {
				return nil, fmt.Errorf("timed out waiting for pre-checks of feature %s (last error: %w)", feature, lastErr)
			}
			return nil, fmt.Errorf("timed out waiting for pre-checks of feature %s", feature)
		}
		var status FeatureDeploymentStatus
		_, err := c.Get(ctx, "/ssp/lcm/features/"+feature+"/status", &status)
		if err != nil {
			// Transient errors (network blips, brief 5xx) shouldn't abort a
			// multi-minute precheck run; keep polling until the deadline.
			lastErr = fmt.Errorf("error polling pre-check status for %s: %w", feature, err)
		} else {
			switch status.PrecheckResults.OverallStatus {
			case "SUCCESS", "SUCCESS_WITH_WARNINGS", "FAILED":
				return &status, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(defaultPollInterval):
		}
	}
}

// WaitForFeatureDeployment polls GET /ssp/lcm/features/{feature}/status until
// the overall_status reaches a terminal deploy or undeploy state.
func (c *Client) WaitForFeatureDeployment(ctx context.Context, feature string) (*FeatureDeploymentStatus, error) {
	deadline := time.Now().Add(defaultPollTimeout)
	var lastErr error
	for {
		if time.Now().After(deadline) {
			if lastErr != nil {
				return nil, fmt.Errorf("timed out waiting for deployment of feature %s (last error: %w)", feature, lastErr)
			}
			return nil, fmt.Errorf("timed out waiting for deployment of feature %s", feature)
		}
		var status FeatureDeploymentStatus
		_, err := c.Get(ctx, "/ssp/lcm/features/"+feature+"/status", &status)
		if err != nil {
			// Transient errors shouldn't abort a multi-minute deployment run;
			// keep polling until the deadline.
			lastErr = fmt.Errorf("error polling deployment status for %s: %w", feature, err)
		} else {
			switch status.OverallStatus {
			case "DEPLOYMENT_SUCCESSFUL", "DEPLOYMENT_PARTIAL_SUCCESS",
				"DEPLOYMENT_FAILED", "PRECHECKS_FAILED",
				"NOT_DEPLOYED", "UNDEPLOYMENT_FAILED", "UNKNOWN":
				return &status, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(defaultPollInterval):
		}
	}
}

// ── Async polling helpers ─────────────────────────────────────────────────────

// WaitForSiteReady polls GET /ssp/sites/{id} until the site reaches
// READY state (connection_status == HEALTHY) or a terminal failure is detected.
func (c *Client) WaitForSiteReady(ctx context.Context, siteID string) error {
	deadline := time.Now().Add(defaultPollTimeout)
	var lastErr error
	for {
		if time.Now().After(deadline) {
			if lastErr != nil {
				return fmt.Errorf("timed out waiting for site %s to become READY (last error: %w)", siteID, lastErr)
			}
			return fmt.Errorf("timed out waiting for site %s to become READY", siteID)
		}

		var site Site
		httpStatus, err := c.Get(ctx, "/ssp/sites/"+siteID, &site)
		if err != nil {
			if httpStatus == 404 {
				// 404 during a readiness poll is a genuine terminal failure
				// (the site was removed), not a transient blip -- fail fast.
				return fmt.Errorf("site %s not found during readiness poll", siteID)
			}
			// Other errors (network blips, brief 5xx) shouldn't abort a
			// multi-minute readiness wait; keep polling until the deadline.
			lastErr = fmt.Errorf("error polling site status: %w", err)
		} else {
			// The API uses current_state == READY or connection_status == HEALTHY.
			if site.CurrentState == "READY" {
				return nil
			}
			if site.Status != nil && site.Status.ConnectionStatus == "HEALTHY" {
				return nil
			}
			// The real SiteReadiness enum (current_state) has two genuine
			// terminal-failure values, NOT_READY and INACTIVE; failing fast on
			// them (rather than spinning for the full 90-minute timeout) mirrors
			// how a 404 above is already treated as terminal. UNKNOWN,
			// ONBOARD_IN_PROGRESS, and OFFBOARD_IN_PROGRESS are legitimate
			// non-terminal states, so those keep polling.
			switch site.CurrentState {
			case "NOT_READY", "INACTIVE":
				return fmt.Errorf("site %s reached a terminal failure state: current_state=%s", siteID, site.CurrentState)
			}
			// Similarly, connection_status == UNHEALTHY is not always a
			// permanent failure on its own (current_state is the authoritative
			// readiness signal), so it does not short-circuit the poll here.
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(defaultPollInterval):
		}
	}
}

// WaitForSiteDeleted polls GET /ssp/sites/{id} until a 404 is returned.
func (c *Client) WaitForSiteDeleted(ctx context.Context, siteID string) error {
	deadline := time.Now().Add(defaultPollTimeout)
	var lastErr error
	for {
		if time.Now().After(deadline) {
			if lastErr != nil {
				return fmt.Errorf("timed out waiting for site %s to be deleted (last error: %w)", siteID, lastErr)
			}
			return fmt.Errorf("timed out waiting for site %s to be deleted", siteID)
		}

		var site Site
		httpStatus, err := c.Get(ctx, "/ssp/sites/"+siteID, &site)
		if httpStatus == 404 {
			return nil
		}
		if err != nil {
			// Transient errors shouldn't abort a multi-minute deletion wait;
			// keep polling until the deadline.
			lastErr = fmt.Errorf("error polling site deletion: %w", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(defaultPollInterval):
		}
	}
}

// WaitForBackupComplete polls GET /ssp/backup/status/{id} until backup reaches terminal state.
func (c *Client) WaitForBackupComplete(ctx context.Context, backupID string) (*BackupStatus, error) {
	deadline := time.Now().Add(defaultPollTimeout)
	var lastErr error
	for {
		if time.Now().After(deadline) {
			if lastErr != nil {
				return nil, fmt.Errorf("timed out waiting for backup %s to complete (last error: %w)", backupID, lastErr)
			}
			return nil, fmt.Errorf("timed out waiting for backup %s to complete", backupID)
		}

		var status BackupStatus
		_, err := c.Get(ctx, "/ssp/backup/status/"+backupID, &status)
		if err != nil {
			// Transient errors shouldn't abort a long-running backup wait;
			// keep polling until the deadline.
			lastErr = fmt.Errorf("error polling backup status: %w", err)
		} else {
			switch status.Status {
			case "SUCCESS":
				return &status, nil
			case "FAILED", "COMPLETED_WITH_ERRORS":
				return &status, fmt.Errorf("backup failed with status: %s", status.Status)
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(defaultPollInterval):
		}
	}
}

// WaitForRestoreComplete polls GET /ssp/restore/status/{id} until restore reaches terminal state.
func (c *Client) WaitForRestoreComplete(ctx context.Context, restoreID string) (*RestoreStatus, error) {
	deadline := time.Now().Add(defaultPollTimeout)
	var lastErr error
	for {
		if time.Now().After(deadline) {
			if lastErr != nil {
				return nil, fmt.Errorf("timed out waiting for restore %s to complete (last error: %w)", restoreID, lastErr)
			}
			return nil, fmt.Errorf("timed out waiting for restore %s to complete", restoreID)
		}

		var status RestoreStatus
		_, err := c.Get(ctx, "/ssp/restore/status/"+restoreID, &status)
		if err != nil {
			// Transient errors shouldn't abort a long-running restore wait;
			// keep polling until the deadline.
			lastErr = fmt.Errorf("error polling restore status: %w", err)
		} else {
			switch status.Status {
			case "SUCCESS":
				return &status, nil
			case "FAILED", "COMPLETED_WITH_ERRORS":
				return &status, fmt.Errorf("restore failed with status: %s", status.Status)
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(defaultPollInterval):
		}
	}
}

// WaitForUpgradeStatus polls GET /ssp/upgrade/status until overall_status
// leaves IN_PROGRESS, returning as soon as any terminal-or-stable state
// (SUCCESS, SUCCESS_WITH_WARNINGS, FAILED, PAUSED, NOT_STARTED) is observed.
// Unlike WaitForUpgradeComplete, it does not treat FAILED as an error — the
// caller inspects OverallStatus to decide the next action (e.g. RETRY).
func (c *Client) WaitForUpgradeStatus(ctx context.Context) (*UpgradeStatus, error) {
	deadline := time.Now().Add(defaultPollTimeout)
	var lastErr error
	var lastStatus UpgradeStatus
	for {
		var status UpgradeStatus
		_, err := c.Get(ctx, "/ssp/upgrade/status", &status)
		if err != nil {
			// Transient errors shouldn't abort a long-running upgrade wait;
			// keep polling until the deadline.
			lastErr = fmt.Errorf("error polling upgrade status: %w", err)
		} else {
			lastErr = nil
			lastStatus = status
			switch status.OverallStatus {
			case "SUCCESS", "SUCCESS_WITH_WARNINGS", "FAILED", "PAUSED", "NOT_STARTED":
				return &status, nil
			}
		}

		if time.Now().After(deadline) {
			if lastErr != nil {
				return nil, fmt.Errorf("timed out waiting for upgrade status to settle (last error: %w)", lastErr)
			}
			return &lastStatus, fmt.Errorf("timed out waiting for upgrade status to settle, last overall_status: %s", lastStatus.OverallStatus)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(defaultPollInterval):
		}
	}
}

// WaitForUpgradeComplete polls GET /ssp/upgrade/status until the upgrade
// reaches a terminal state, returning an error if it fails.
func (c *Client) WaitForUpgradeComplete(ctx context.Context) (*UpgradeStatus, error) {
	status, err := c.WaitForUpgradeStatus(ctx)
	if err != nil {
		return status, err
	}
	switch status.OverallStatus {
	case "SUCCESS", "SUCCESS_WITH_WARNINGS":
		return status, nil
	default:
		return status, fmt.Errorf("upgrade did not complete successfully, overall_status: %s", status.OverallStatus)
	}
}

// ── HTTP primitives ───────────────────────────────────────────────────────────

// do executes an HTTP request with Basic Auth and returns the response body.
func (c *Client) do(ctx context.Context, method, path string, body interface{}) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewBuffer(data)
	}

	url := c.host + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, resp.StatusCode, nil
}

// Get performs a GET request and decodes the JSON response into out.
func (c *Client) Get(ctx context.Context, path string, out interface{}) (int, error) {
	body, status, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return status, err
	}
	if out != nil && len(body) > 0 {
		if err := json.Unmarshal(body, out); err != nil {
			return status, fmt.Errorf("failed to decode response: %w", err)
		}
	}
	return status, nil
}

// Post performs a POST request with a JSON body and decodes the response.
func (c *Client) Post(ctx context.Context, path string, in, out interface{}) (int, error) {
	body, status, err := c.do(ctx, http.MethodPost, path, in)
	if err != nil {
		return status, err
	}
	if out != nil && len(body) > 0 {
		if err := json.Unmarshal(body, out); err != nil {
			return status, fmt.Errorf("failed to decode response: %w", err)
		}
	}
	return status, nil
}

// Put performs a PUT request with a JSON body and decodes the response.
func (c *Client) Put(ctx context.Context, path string, in, out interface{}) (int, error) {
	body, status, err := c.do(ctx, http.MethodPut, path, in)
	if err != nil {
		return status, err
	}
	if out != nil && len(body) > 0 {
		if err := json.Unmarshal(body, out); err != nil {
			return status, fmt.Errorf("failed to decode response: %w", err)
		}
	}
	return status, nil
}

// Delete performs a DELETE request (no body) and returns the status code.
func (c *Client) Delete(ctx context.Context, path string) (int, error) {
	_, status, err := c.do(ctx, http.MethodDelete, path, nil)
	return status, err
}

// DeleteWithBody performs a DELETE request with a JSON body (used by site offboarding).
func (c *Client) DeleteWithBody(ctx context.Context, path string, body interface{}) (int, error) {
	_, status, err := c.do(ctx, http.MethodDelete, path, body)
	return status, err
}

// WaitForSSPAPIReady polls GET /ssp/cluster/monitor/platform/status with provider
// credentials until a successful (2xx) response is received, confirming the SSP
// REST API is fully initialised. Retries every 30 s up to the given timeout.
func (c *Client) WaitForSSPAPIReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for SSP API (/ssp/cluster/monitor/platform/status) to return HTTP 200")
		}
		var status ClusterStatus
		_, err := c.Get(ctx, "/ssp/cluster/monitor/platform/status", &status)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sspAPIReadyPollInterval):
		}
	}
}
