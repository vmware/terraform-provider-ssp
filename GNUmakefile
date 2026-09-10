TEST?=$$(go list ./...)
GOFMT_FILES?=$$(find . -name '*.go')
TESTARGS?=
PKG_NAME=ssp
GIT_COMMIT=$$(git rev-list -1 HEAD 2>/dev/null || echo "unknown")
BUILD_PATH=$$(go env GOPATH)
VERSION?=1.0.0
DIST_DIR:=dist

default: build

tools:
	GO111MODULE=on go install -mod=mod github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2
	GO111MODULE=on go install -mod=mod github.com/katbyte/terrafmt
	GO111MODULE=on go install -mod=mod github.com/jstemmer/go-junit-report/v2@v2.1.0

build: fmtcheck
	mkdir -p $(DIST_DIR)
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(GIT_COMMIT)" -o $(DIST_DIR)/terraform-provider-ssp .

test: fmtcheck
	go test ./... -v -count=1 -parallel=4

testacc: fmtcheck
	GO111MODULE=on TF_ACC=1 go test ./... -v -count=1 -parallel=4

# testacc-ci is the entry point used by the Jenkins acceptance pipeline
# (ci/jenkins/Jenkinsfile.acceptance): same run as `testacc`, but bounded by
# a timeout and converted to JUnit XML for Jenkins' Test Result Trend.
testacc-ci: fmtcheck
	mkdir -p test-results
	GO111MODULE=on TF_ACC=1 go test ./... -v -count=1 -parallel=4 -timeout=180m $(TESTARGS) 2>&1 | tee test-results/testacc.log | go-junit-report -set-exit-code > test-results/junit.xml

vet:
	@echo "go vet ."
	@go vet $$(go list ./... | grep -v vendor/) ; if [ $$? -eq 1 ]; then \
		echo ""; \
		echo "Vet found suspicious constructs. Please check the reported constructs"; \
		echo "and fix them if necessary before submitting the code for review."; \
		exit 1; \
	fi

fmt:
	gofmt -s -w $(GOFMT_FILES)

fmtcheck:
	@sh -c "'$(CURDIR)/scripts/gofmtcheck.sh'"

errcheck:
	@sh -c "'$(CURDIR)/scripts/errcheck.sh'"

test-unit:
	go test -v ./internal/... -tags=unittest -count=1

generate:
	go generate ./...
	@$(MAKE) docs-lint-fix

docs-lint:
	@echo "==> Checking Markdown docs lint..."
	markdownlint-cli2 "docs/**/*.md" --config ".markdownlint.jsonc"

docs-lint-fix:
	@echo "==> Fixing Markdown docs lint..."
	markdownlint-cli2 "docs/**/*.md" --config ".markdownlint.jsonc" --fix

# docs-compat regenerates docs/guides/version-compatibility.md and the
# generated section of README.md from internal/compat/compatibility.yaml.
# CI (.github/workflows/docs-lint.yml) re-runs this and fails if the
# committed docs don't match, so compatibility.yaml stays the single
# source of truth.
docs-compat:
	python3 scripts/gen-compat-docs.py ssp

.PHONY: build test testacc testacc-ci vet fmt fmtcheck errcheck test-unit generate docs-lint docs-lint-fix docs-compat tools default
