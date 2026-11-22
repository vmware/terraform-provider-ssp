# Testing the Terraform Provider for VMware SSP

This guide covers running unit and acceptance tests for the **Terraform Provider for VMware Security Services
Platform (SSP)** (`terraform-provider-ssp`), setting up SOCKS5 proxy access for isolated testbeds, and
troubleshooting common testing and diagnostic issues.

For the companion SSPI appliance provider's test suite, see
[`terraform-provider-sspi`'s testing guide](https://github.com/vmware/terraform-provider-sspi/blob/main/docs/test.md).

---

## Test Suites Overview

The provider test suite consists of two main types of tests:

1. **Unit Tests**: Test resource/datasource schema conversions, state mapping, and client request/response formatting against in-memory HTTP mock servers (`httptest.Server`). They execute quickly and do not require live infrastructure.
2. **Acceptance Tests**: Run real end-to-end Terraform CRUD lifecycle operations against a deployed SSP platform cluster (`vxlan-vm-*.nimbus.internal`).

---

## Running Unit Tests

Unit tests execute locally and perform in-memory HTTP mocking without hitting external network endpoints.

Run all unit tests via `make`:

```shell
make test-unit
```

Or using standard `go test`:

```shell
go test -v ./internal/... -tags=unittest -count=1
```

*Note: The provider HTTP client transport automatically bypasses proxies for `127.0.0.1` and `localhost` to ensure unit test HTTP mock servers run without interference from `ALL_PROXY` settings.*

---

## Running Acceptance Tests

Acceptance tests require network access to a live **SSP Platform Cluster**.

### 1. Environment Variable Credentials

```shell
export SSP_HOST="https://vxlan-vm-111-71.nimbus-tb.nimbus.internal"
export SSP_USERNAME="admin"
export SSP_PASSWORD="your-ssp-password"
export SSP_INSECURE="true"
```

#### Optional variables

No acceptance test is ever skipped. A test that needs more than the platform **fails**, naming the missing
variable, when it is not set, so set all of these for a full run.

| Variable | Enables |
| --- | --- |
| `SSP_ACC_BACKUP_SERVER`, `_USERNAME`, `_PASSWORD`, `_PASSPHRASE`, `_LOCATION`, `_SSH_PUBLIC_KEY` (optional `_PORT`, `_ALT_LOCATION`) | `ssp_backup_config` (an SFTP server) |
| `SSP_ACC_ALLOW_BACKUP` (default true) | `ssp_backup` takes real backups (they cannot be deleted afterwards) |
| `SSP_ACC_ALLOW_RESTORE` (default true) | `ssp_restore` restores the whole platform |
| `SSP_ACC_ALLOW_BAD_CREDENTIALS` (default true) | sends a wrong password (can lock the admin account for some minutes) |
| `SSP_ACC_TEST_FEATURE` | `ssp_feature` deploy and undeploy (default `INTELLIGENCE`; `TF_ACC` defaults to `1` for these tests) |
| `SSP_ACC_MALWARE_CLOUD_UNSUPPORTED` (default true) | CLOUD analysis mode is rejected (only on a deployment without cloud connectivity) |
| `NSX_HOST`, `NSX_USERNAME`, `NSX_PASSWORD`, `NSX_CERTIFICATE`, `NSX_SITE_NAME` | `ssp_site` (an NSX Manager that is not onboarded yet) |
| `SSP_ACC_ALLOW_SITE_ONBOARD` (default true) | sends onboarding requests for that NSX Manager |

Most resources here are singletons: the tests change platform settings, leave them on the defaults noted
in each test, and cannot delete them. Do not run the suite against a cluster whose settings matter. They
also use plain `resource.Test`, not `ParallelTest`, so they run one at a time.

Test helpers call the platform through the provider's own client (`internal/provider/client`), so they
exercise the same transport, proxy and TLS handling as the provider.

### 2. Executing Acceptance Tests

To enable acceptance testing, set `TF_ACC=1`:

```shell
make testacc
```

`make testacc` allows 120 minutes (`-timeout 120m`); the full suite takes about 20 minutes.

#### Running Specific Acceptance Tests

To run a specific test suite or test case, pass the `TESTARGS` variable:

```shell
make testacc TESTARGS="-run=TestAccSiteDataSource"
```

To run a single test case directly with `go test`:

```shell
TF_ACC=1 go test -v ./internal/provider -run=TestAccSensorRegistrationTokenResource -timeout 5m
```

---

## Running Acceptance Tests Before Submitting a PR

There's no automatic per-PR acceptance-test trigger yet. Until then, this is how to self-check a
change against a real SSP platform cluster before opening or updating a PR:

### Option 1 — Run locally

```shell
export SSP_HOST="https://<your-testbed>"
export SSP_USERNAME="admin"
export SSP_PASSWORD="..."
export SSP_INSECURE="true"
make testacc
```

Fastest feedback loop — no CI latency, and you see failures in your own terminal.

### Option 2 — Run via GitHub Actions against your branch

The "Acceptance Tests" workflow (`.github/workflows/acceptance-tests.yml`) supports
`workflow_dispatch`, which can run from **any branch**, including a feature branch before a PR
exists:

1. Push your branch.
2. Actions tab → "Acceptance Tests" → **Run workflow** → pick your branch.
3. Fill in `ssp_host`/`ssp_username`/`ssp_password` in the form. If your change touches `ssp_site`,
   also fill in `nsx_host`/`nsx_username`/`nsx_password`/`nsx_certificate` — those tests need a
   real NSX Manager and fail without them. The other optional variables above come from
   repository secrets and variables of the same names (for example `SSP_ACC_BACKUP_SERVER`,
   `SSP_ACC_ALLOW_BACKUP`).
4. Check the run's summary and log; once it's green, open or update the PR.

This gives you a logged, shareable CI run instead of just local terminal output, but you still have
to trigger it yourself each time — it isn't wired to fire automatically on push or PR open.

### What happens if nothing is configured

With `TF_ACC=1` the suite **fails** when `SSP_HOST`, `SSP_USERNAME` or `SSP_PASSWORD` is empty:
`testAccSSPRuntimePreCheck` (`internal/provider/provider_test.go`) calls `t.Fatalf` naming the missing
variable, so an unconfigured run can never look green. Tests that need the variables in the table above fail the same way, naming what is missing. If `TF_ACC` itself is
not set, `terraform-plugin-testing` skips every acceptance test before the PreCheck runs.

The `ssp_site` tests call `testAccNSXSitePreCheck`, which fails without the `NSX_*` variables, and the
site tests also fail if that NSX Manager is already onboarded.

---

## SOCKS5 Proxy & Testbed Connectivity Setup

When running acceptance tests against isolated lab environments (e.g., Nimbus testbeds accessible only via a jump host), route HTTP traffic through an SSH SOCKS5 proxy tunnel.

### 1. Establishing the SSH SOCKS5 Tunnel

Open a SOCKS5 proxy on local port `9999` using the jump host:

```shell
ssh worker@<jump_host_ip> -D 9999 -N
```

### 2. Proxy Environment Variables (`socks5h://`)

Configure proxy environment variables before running acceptance tests. **Crucially, use `socks5h://` (with `h`) to mandate remote DNS resolution on the jump host:**

```shell
export ALL_PROXY="socks5h://127.0.0.1:9999"
export HTTPS_PROXY="socks5h://127.0.0.1:9999"
export HTTP_PROXY="socks5h://127.0.0.1:9999"
export NO_PROXY="127.0.0.1,localhost,::1"
```

---

## Troubleshooting & Testing Diagnosis Guide

| Symptoms / Error Message | Root Cause | Resolution / Fix |
| --- | --- | --- |
| `dial tcp: lookup <hostname>: no such host` | Using standard `socks5://` scheme causing local DNS lookup for internal domain names (e.g. `.nimbus.internal`). | Change proxy protocol scheme to `socks5h://` (e.g., `ALL_PROXY="socks5h://127.0.0.1:9999"`). The `h` suffix forces DNS resolution via the remote SOCKS proxy. |
| `socks connect tcp 127.0.0.1:9999->127.0.0.1:XXXXX: dial tcp ... connection refused` | `ALL_PROXY` environment variable routing local `httptest.Server` unit test calls into the SOCKS proxy. | Ensure `export NO_PROXY="127.0.0.1,localhost"` is set. The provider client automatically disables proxying for `127.0.0.1` and `localhost` targets. |
| `Passphrase does not meet the SSP password policy` / HTTP 400 | Passphrase or password string supplied in test config fails SSP platform complexity rules. | Use a compliant password containing uppercase, lowercase, numbers, and special characters with minimum length of 15 (e.g. `SecretPassphrase123!`). |
| `Attribute 'telemetry_collector_instance_id' expected to be set` | The platform `PUT /ssp/telemetry/config` endpoint does not echo back generated read-only fields in the response body. | The provider handles this by automatically performing a follow-up `GET` request after `PUT` to refresh state attributes. |
| `ssh: connect to host ... port 22: Operation timed out` | Jump host IP or port 22 is blocked by local firewall or VPN routing. | Verify network route to jump host and ensure corporate VPN or host route is active. |
