# Contributing to Terraform Provider for SSP

Thank you for your interest in contributing to the Terraform Provider for SSP!

## Getting Started

1. Ensure you have Go 1.22+ installed.
2. Clone the repository.
3. Run `make build` to compile the provider binary.
4. Run `make fmt` and `make docs-lint-fix` before opening a pull request.
5. Run `golangci-lint run` to verify linter compliance.

## Pull Request Guidelines

- Ensure unit tests pass (`go test ./...`).
- Format all code with `gofmt`.
- Update documentation in `docs/` as necessary.
