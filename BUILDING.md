# Building the Terraform Provider for VMware SSP

Instructions for building and developing the **Terraform Provider for VMware Security Services Platform (SSP)** (`terraform-provider-ssp`).

---

## Requirements

Before building the provider, ensure you have installed:

- [Go](https://go.dev/dl/) ≥ 1.21 (1.25+ recommended)
- [Terraform](https://developer.hashicorp.com/terraform/downloads) ≥ 1.5 (for local testing with HCL configurations)
- [Git](https://git-scm.com/)

---

## Building the Provider Binary

1. Clone the repository:

   ```shell
   git clone https://github.com/vmware/terraform-provider-ssp.git
   cd terraform-provider-ssp
   ```

2. Build the provider using `make`:

   ```shell
   make build
   ```

   This outputs the compiled binary to the `dist/` directory: `dist/terraform-provider-ssp`.

   Alternatively, build directly using Go:

   ```shell
   go build -o terraform-provider-ssp .
   ```

---

## Development Overrides (`~/.terraformrc`)

To test local provider builds with Terraform CLI without publishing to a registry, configure **Development Overrides** in your `~/.terraformrc` file (or `%APPDATA%\terraform.rc` on Windows):

```hcl
provider_installation {
  dev_overrides {
    "registry.terraform.io/vmware/ssp" = "/Users/<your-username>/go/src/github.com/terraform-providers/terraform-provider-ssp/dist"
  }

  direct {}
}
```

Replace the directory path with the absolute path to your local output directory containing the built `terraform-provider-ssp` binary.

---

## Installing the Plugin Locally

To install the plugin binary into your local user plugin directory (`~/.terraform.d/plugins`):

```shell
make install
```
