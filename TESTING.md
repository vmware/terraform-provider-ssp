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

### 2. Executing Acceptance Tests

To enable acceptance testing, set `TF_ACC=1`:

```shell
make testacc
```

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
   real NSX Manager and skip cleanly without them.
4. Check the run's summary and log; once it's green, open or update the PR.

This gives you a logged, shareable CI run instead of just local terminal output, but you still have
to trigger it yourself each time — it isn't wired to fire automatically on push or PR open.

### What happens if nothing is configured

Both options above **skip cleanly rather than failing** if the connection details aren't set.
`testAccSSPRuntimePreCheck` and `testAccNSXSitePreCheck` (`internal/provider/provider_test.go`)
call `t.Skip(...)` if `SSP_HOST`/`SSP_USERNAME`/`SSP_PASSWORD` (or, for `ssp_site` tests,
`NSX_HOST`/`NSX_USERNAME`/`NSX_PASSWORD`/`NSX_CERTIFICATE`) are empty — every `TestAcc*` test
reports `SKIP` with a message naming the missing variable, and the overall run still reports
success (exit code 0). Separately, if `TF_ACC` itself isn't set, the `terraform-plugin-testing`
framework skips every acceptance test before the PreCheck even runs, for the same reason.

The one case that **does** fail rather than skip: `SSP_HOST` is set but wrong or unreachable —
then the test gets past the `t.Skip()` checks and fails with a real connection/HTTP error. So a
green "Acceptance Tests" run with all tests reported `SKIP` means "nothing was actually
configured," not "the tests passed" — check the log, not just the overall pass/fail status.

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
