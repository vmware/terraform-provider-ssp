# Terraform Provider for VMware Security Services Platform (SSP)

This is the official Terraform provider for the **VMware Security Services Platform (SSP)** runtime cluster
(`terraform-provider-ssp`).

It provides Day-2 Infrastructure-as-Code (IaC) automation for a **deployed** SSP cluster (`provider "ssp"`):

1. **Site Onboarding**: Onboarding, reconnecting, or offboarding NSX Manager / Avi sites (`ssp_site`).
2. **Vertical Security Activation**: Activating modular vDefend security verticals (`ssp_feature`: Security
   Intelligence, NDR, Malware Prevention, Cloud Connector, AI Assist, Rule Analysis), Cloud Connector region
   selection (`ssp_cloud_connector_config`), and NDR data-sharing (`ssp_ndr_config`).
3. **Cluster Operations**: Telemetry/CEIP (`ssp_telemetry_config`), runtime cluster backups/restores
   (`ssp_backup_config`, `ssp_backup`, `ssp_restore`), and in-place cluster upgrades (`ssp_upgrade`).

For Day-0/Day-1 SSPI appliance workflows (vCenter provider registration, package management, cluster
provisioning and scaling, LDAP/local user management), see the companion
[`terraform-provider-sspi`](https://github.com/vmware/terraform-provider-sspi) provider — the two are
designed to be used together within the same root module via `depends_on`. See the
[FSDD](FSDD.md) for the full architecture and the cross-provider dependency model.

---

## Requirements

| Dependency | Version |
| --- | --- |
| [Terraform](https://developer.hashicorp.com/terraform/downloads) | ≥ 1.5 |
| [Go](https://go.dev/dl/) | ≥ 1.21 (only for building from source) |
| SSP Platform | Accessible over HTTPS (`port 443`), admin credentials available |

<!-- BEGIN GENERATED: version-compatibility (see internal/compat/compatibility.yaml, scripts/gen-compat-docs.py) -->
**Platform compatibility:** tested against platform version `5.2.0`, minimum supported `5.2.0`. See [Version Compatibility](docs/guides/version-compatibility.md) for details.
<!-- END GENERATED: version-compatibility -->

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

```shell
export SSP_HOST="https://ssp-cluster.corp.local"
export SSP_USERNAME="admin"
export SSP_PASSWORD="your-ssp-password"
```

```hcl
provider "ssp" {
  host     = "https://ssp-cluster.corp.local" # env: SSP_HOST
  username = "admin"                          # env: SSP_USERNAME
  password = var.ssp_password                 # env: SSP_PASSWORD
  insecure = true                             # set true to skip TLS verification (labs)
}
```

**Required role:** the configured account must hold the `enterprise_admin` role. Nearly every
write operation (create/update/delete on any `ssp_*` resource) requires `enterprise_admin`, alone
or paired with `auditor`; an `auditor`-only or `security_op`-only account can read via the
`data.ssp_*` data sources but will fail with an HTTP 403 on the first resource write.

---

## End-to-End HCL Deployment Example

This example onboards an NSX Manager site, activates vDefend features, and configures backup settings against
an already-deployed SSP cluster. If the cluster itself still needs to be provisioned, see the companion
[`terraform-provider-sspi`](https://github.com/vmware/terraform-provider-sspi) provider and add a
`depends_on` from `ssp_site` to the `sspi_platform` resource:

```hcl
terraform {
  required_providers {
    ssp = {
      source  = "registry.terraform.io/vmware/ssp"
      version = "1.0.0"
    }
  }
}

provider "ssp" {
  host     = "https://ssp-cluster.corp.local"
  username = "admin"
  password = var.ssp_password
  insecure = true
}

resource "ssp_site" "nsx_site" {
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

## Resources & Data Sources

| Resource | Description |
| --- | --- |
| `ssp_site` | Onboards, reconnects, or offboards NSX Manager / Avi sites (`/ssp/sites`) |
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

| Data Source | Description |
| --- | --- |
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

- 🛠️ [**Build & Installation Guide**](BUILDING.md): Details on building binaries, local provider installation (`~/.terraform.d/plugins`), and Terraform CLI development overrides (`~/.terraformrc`).
- 🧪 [**Testing & Diagnosis Guide**](TESTING.md): Instructions for unit tests (`make test-unit`), acceptance tests (`make testacc`), testbed environment variables, SSH SOCKS5 proxy configuration (`socks5h://`), and common troubleshooting scenarios.

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

Acceptance tests require credentials to a live SSP cluster. When testing isolated testbeds over an SSH SOCKS5 tunnel (`127.0.0.1:9999`), set `socks5h://` in proxy environment variables to enable remote DNS resolution:

```shell
# 1. Establish SSH SOCKS5 tunnel to lab jump host
ssh worker@<jump_host_ip> -D 9999 -N

# 2. Run acceptance tests via proxy
export TF_ACC=1
export ALL_PROXY="socks5h://127.0.0.1:9999"
export HTTPS_PROXY="socks5h://127.0.0.1:9999"
export HTTP_PROXY="socks5h://127.0.0.1:9999"
export NO_PROXY="127.0.0.1,localhost,::1"

export SSP_HOST="https://vxlan-vm-111-71.nimbus-tb.nimbus.internal"
export SSP_USERNAME="admin"
export SSP_PASSWORD="your-ssp-password"

make testacc
```
