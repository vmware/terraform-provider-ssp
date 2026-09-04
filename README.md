# Terraform Provider for VMware Security Services Platform (SSP & SSPI)

This is the official Terraform provider for **VMware Security Services Platform (SSP)** and **Security Services Platform Installer (SSPI)** (`terraform-provider-ssp`).

It provides end-to-end Infrastructure-as-Code (IaC) automation across the entire platform lifecycle within a single provider (`provider "ssp"`):

1. **Day-0 Bootstrapping & Target Setup**: Registering vCenter providers (`ssp_vsphere_provider`), uploading `.tar.gz` software packages to the SSPI Depot (`ssp_bundle_local`), setting up SSPI appliance backup targets (`ssp_installer_backup_config`), and configuring LDAP identity sources (`ssp_ldap_identity_source`).
2. **Day-1 Cluster Deployment & Lifecycle**: Pre-deployment validation, provisioning SSP (`ATP`) and Avi Operations clusters, long-running async lifecycle polling, scaling worker nodes/form factors, updating IP pools, teardown (`ssp_platform`), and local password management (`ssp_user_password`).
3. **Day-2 Runtime Operations & Vertical Activation**: Onboarding NSX Manager sites (`ssp_site`), activating modular vDefend security verticals (`ssp_feature`: Security Intelligence, NDR, Malware Prevention, Cloud Connector, AI Assist, Rule Analysis), Cloud Connector region selection (`ssp_cloud_connector_config`), NDR data-sharing (`ssp_ndr_config`), Telemetry/CEIP (`ssp_telemetry_config`), runtime cluster backups/restores (`ssp_backup_config`, `ssp_backup`, `ssp_restore`), and in-place cluster upgrades (`ssp_upgrade`).

---

## Requirements

| Dependency | Version |
|---|---|
| [Terraform](https://developer.hashicorp.com/terraform/downloads) | ≥ 1.5 |
| [Go](https://go.dev/dl/) | ≥ 1.21 (only for building from source) |
| SSPI Appliance / SSP Platform | Accessible over HTTPS (`port 443`), admin credentials available |

---

## Installation

Add the provider to your `required_providers` block:

```hcl
terraform {
  required_providers {
    ssp = {
      source  = "registry.terraform.io/vmware/ssp"
      version = "1.0.0"
    }
  }
  required_version = ">= 1.5"
}
```

Then run:

```shell
terraform init
```

---

## Authentication & Provider Configuration

The provider supports connecting to both the **SSPI Appliance** (for Day-0/1 installer workflows) and the **SSP Runtime Cluster** (for Day-2 operational workflows). Credentials can be supplied via HCL attributes or environment variables:

```shell
# SSPI Appliance Credentials (Day-0 / Day-1)
export SSPI_HOST="https://sspi.corp.local"
export SSPI_USERNAME="admin"
export SSPI_PASSWORD="your-sspi-password"

# SSP Cluster Credentials (Day-2)
export SSP_HOST="https://ssp-cluster.corp.local"
export SSP_USERNAME="admin"
export SSP_PASSWORD="your-ssp-password"
```

### Provider Configuration Reference

```hcl
provider "ssp" {
  # --- SSPI Appliance Connection (Day-0 / Day-1) ---
  sspi_host     = "https://sspi.corp.local"   # env: SSPI_HOST
  sspi_username = "admin"                     # env: SSPI_USERNAME
  sspi_password = var.sspi_password           # env: SSPI_PASSWORD
  sspi_insecure = true                        # set true to skip TLS verification (labs)

  # --- SSP Runtime Cluster Connection (Day-2) ---
  host          = "https://ssp-cluster.corp.local" # env: SSP_HOST
  username      = "admin"                           # env: SSP_USERNAME
  password      = var.ssp_password                  # env: SSP_PASSWORD
  insecure      = true                              # set true to skip TLS verification (labs)
}
```

---

##  End-to-End HCL Deployment Example

This end-to-end HCL configuration registers a vCenter provider, uploads a software package bundle, deploys an SSP cluster, onboards an NSX Manager site, activates vDefend features, and configures cloud & backup settings :

```hcl
# ------------------------------------------------------------------------------
# 1.  Provider Declaration
# ------------------------------------------------------------------------------
terraform {
  required_providers {
    ssp = {
      source  = "registry.terraform.io/vmware/ssp"
      version = "1.0.0"
    }
  }
}

provider "ssp" {
  # --- Day-2 SSP Runtime Cluster Configuration ---
  host     = "https://ssp-cluster.corp.local"
  username = "admin"
  password = var.ssp_password
  insecure = true

  # --- Day-0/1 SSP Installer Configuration ---
  sspi_host     = "https://sspi.corp.local"
  sspi_username = "admin"
  sspi_password = var.sspi_password
  sspi_insecure = true
}

# ------------------------------------------------------------------------------
# 2. Day-0/1 Provisioning Workflows (vCenter, Package Depot, Platform Deployment)
# ------------------------------------------------------------------------------
resource "ssp_vsphere_provider" "vc01" {
  server      = "vc01.corp.local"
  user        = "administrator@vsphere.local"
  password    = var.vc_password
  certificate = var.vc_certificate
}

resource "ssp_installer_bundle_local" "ssp_pkg" {
  file_path = "/tmp/ssp-platform-5.2.0.tar.gz"
}

resource "ssp_platform" "ssp_cluster" {
  provider_id          = ssp_vsphere_provider.vc01.id
  display_name         = "Production-SSP-Cluster"
  ssp_type             = "ATP"
  form_factor          = "MEDIUM" # real enum: COMPACT | MEDIUM | LARGE | EXTRA_LARGE
  worker_count         = 3
  domain               = "corp.local"
  datacenter_id        = "datacenter-1"
  cluster_id           = "domain-c1"
  content_datastore_id = "datastore-1"
  network_id           = "dvportgroup-1"
  storage_policy_id    = var.vsan_storage_policy_id
  platform_default_gateway = "10.0.0.1"
  platform_subnet          = "10.0.0.0/24"
  dns_servers          = ["10.0.0.10"]
  ntp_server           = "ntp.corp.local"
  node_ip_pool         = ["172.16.111.50-172.16.111.60"]
  service_ip_pool      = ["172.16.111.70-172.16.111.80"]
  ingress_fqdn         = "ssp-cluster.corp.local"
  kafka_fqdn           = "ssp-cluster-kafka.corp.local"
  ssp_bundle_id        = ssp_installer_bundle_local.ssp_pkg.id
  admin_password       = var.ssp_admin_password
  audit_password       = var.ssp_audit_password
}

# ------------------------------------------------------------------------------
# 3. Day-2 Operational Workflows (NSX Site, Features, SIEM, Backup)
# ------------------------------------------------------------------------------
resource "ssp_site" "nsx_site" {
  depends_on = [ssp_platform.ssp_cluster]

  site_name     = "NSX-Manager-Prod"
  site_type     = "NSX_MANAGER"
  desired_state = "ONBOARD" # real enum: ONBOARD | OFFBOARD | PREPARE (not "ONBOARDED")
  force         = false     # honored on both onboard and offboard

  site_connection_info = {
    connection_type = "DYNAMIC"
    hostname        = "nsx01.corp.local"
    username        = "admin"
    password        = var.nsx_password
    certificate     = var.nsx_ca_cert
  }
}

resource "ssp_feature" "ndr" {
  depends_on = [ssp_site.nsx_site]
  feature    = "NDR"
}

# ssp_ndr_config exposes a single data-sharing toggle — the real
# /ssp/lcm/ndr/config API has no syslog/SIEM forwarding fields.
resource "ssp_ndr_config" "ndr_data_sharing" {
  data_sharing = true
}

# ssp_cloud_connector_config selects a Cloud Connector analytics region — the
# real /ssp/lcm/cloud-connector/config API has no HTTP/web-proxy fields.
resource "ssp_cloud_connector_config" "cloud_connector" {
  region = "west.us"
}

resource "ssp_backup_config" "sftp_target" {
  server_address  = "backup.corp.local"
  protocol        = "SFTP"
  port            = 22
  username        = "sftpuser"
  password        = var.sftp_password
  backup_location = "/backups/ssp-cluster"
  ssh_public_key  = var.sftp_host_key
}
```

---

## Resources & Data Sources Taxonomy

### SSPI Installer Resources (Day-0 / Day-1)

| Resource | Description |
|---|---|
| `ssp_vsphere_provider` | Registers and manages vCenter target provider endpoints (`/sspi/providers`) |
| `ssp_installer_bundle_local` | Uploads `.tar.gz` software packages into SSPI Depot (`/sspi/bundles`) |
| `ssp_platform` | Provision, scale, reconfigure, and teardown SSP or Avi Operations clusters (`/sspi/platforms`) |
| `ssp_installer_backup_config` | Configures SSPI appliance SFTP backup target (`/sspi/backup/config`) |
| `ssp_installer_recurring_backup_config` | Configures SSPI appliance recurring backup schedule (`/sspi/backup/recurring/config`) |
| `ssp_ldap_identity_source` | Configures SSPI appliance and runtime LDAP directory integration (`/sspi/iam/ldap-identity-sources`) |
| `ssp_user_password` | Manages local user account password resets and changes (`/sspi/iam/*`) |

### SSP Runtime Resources (Day-2)

| Resource | Description |
|---|---|
| `ssp_site` | Onboards, reconnects, or offboards NSX Manager / Avi sites (`/ssp/site-service/sites`) |
| `ssp_feature` | Activates or deactivates vDefend security verticals (`/ssp/lcm/features/{feature}`) |
| `ssp_cloud_connector_config` | Selects Cloud Connector analytics region (`/ssp/lcm/cloud-connector/config`) |
| `ssp_ndr_config` | Toggles NDR cloud data-sharing (`/ssp/lcm/ndr/config`) |
| `ssp_malware_prevention_config` | Configures on-premises malware analysis engine (`/ssp/lcm/malware-prevention/config`) |
| `ssp_telemetry_config` | Configures CEIP and telemetry collection schedule (`/ssp/telemetry/config`) |
| `ssp_sensor_registration_token` | Creates host sensor registration tokens (`/sensors/registration-tokens`) |
| `ssp_backup_config` | Configures runtime cluster SFTP backup target (`/ssp/backup/config`) |
| `ssp_recurring_backup_config` | Configures runtime cluster recurring backup schedule (`/ssp/backup/recurring/config`) |
| `ssp_backup` | Triggers ad-hoc runtime cluster backup (`/ssp/backup`) |
| `ssp_restore` | Restores runtime cluster state from backup (`/ssp/restore`) |
| `ssp_upgrade` | Initiates and tracks in-place cluster system upgrades (`/ssp/upgrade`) |
| `ssp_readiness` | Checks platform operational readiness (`/ssp/cluster/monitor/platform/status`) |

### Data Sources

| Data Source | Description |
|---|---|
| `data.ssp_vsphere_provider` | Fetches details of a registered vCenter provider |
| `data.ssp_platform` | Reads SSPI platform configuration and status |
| `data.ssp_bundle` | Reads SSPI Depot package bundle metadata |
| `data.ssp_platform_status` | Retrieves comprehensive SSP cluster health, node count, and version |
| `data.ssp_feature_health` | Retrieves cluster feature health status |
| `data.ssp_sites` | Lists all onboarded sites |
| `data.ssp_site` | Reads details of an onboarded site |
| `data.ssp_licenses` | Queries platform product licenses |
| `data.ssp_sensors` | Queries registered security sensors |
| `data.ssp_backup_status` | Queries runtime backup job status |
| `data.ssp_feature` | Queries feature module deployment state |
| `data.ssp_upgrade_available_versions` | Lists target versions available for upgrade |
| `data.ssp_upgrade_history` | Lists past upgrade execution records |

---

## Development, Testing & Diagnosis

For comprehensive guides on building, installing, running unit and acceptance tests, setting up SSH SOCKS5 proxies, and troubleshooting diagnostic issues, refer to:

- 🛠️ [**Build & Installation Guide**](docs/build.md): Details on building binaries, local provider installation (`~/.terraform.d/plugins`), and Terraform CLI development overrides (`~/.terraformrc`).
- 🧪 [**Testing & Diagnosis Guide**](docs/test.md): Instructions for unit tests (`make test-unit`), acceptance tests (`make testacc`), testbed environment variables, SSH SOCKS5 proxy configuration (`socks5h://`), and common troubleshooting scenarios.

### Quick Commands

#### Building from Source

```shell
make build
```

#### Running Unit Tests

Unit tests perform in-memory HTTP mocking without requiring live testbeds:

```shell
make test-unit
```

#### Running Acceptance Tests

Acceptance tests require credentials to a live SSPI appliance and/or SSP cluster. When testing isolated testbeds over an SSH SOCKS5 tunnel (`127.0.0.1:9999`), set `socks5h://` in proxy environment variables to enable remote DNS resolution:

```shell
# 1. Establish SSH SOCKS5 tunnel to lab jump host
ssh worker@<jump_host_ip> -D 9999 -N

# 2. Run acceptance tests via proxy
export TF_ACC=1
export ALL_PROXY="socks5h://127.0.0.1:9999"
export HTTPS_PROXY="socks5h://127.0.0.1:9999"
export HTTP_PROXY="socks5h://127.0.0.1:9999"
export NO_PROXY="127.0.0.1,localhost,::1"

export SSPI_HOST="https://172.16.111.4"
export SSPI_USERNAME="admin"
export SSPI_PASSWORD="your-sspi-password"

export SSP_HOST="https://vxlan-vm-111-71.nimbus-tb.nimbus.internal"
export SSP_USERNAME="admin"
export SSP_PASSWORD="your-ssp-password"

make testacc
```
