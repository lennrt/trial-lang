GO_VERSION := 1.27.0
GOLANGCI_LINT_VERSION := v2.13.2
GOIMPORTS_VERSION := v0.49.0
GOVULNCHECK_VERSION := v1.7.0
GO_LICENSES_VERSION := v2.0.1
ACTIONLINT_VERSION := v1.7.12
OPENSPEC_VERSION := 1.13.2
NPX ?= npx
FUZZ_TIME ?= 30s
FUZZ_TIMEOUT ?= 2m

.PHONY: hooks build test race property fuzz coverage fmt fmt-check vet lint tidy-check examples api-check workflow-check vuln licenses purego arm64 demo-generate demo-check verify

hooks:
	git config core.hooksPath .githooks
	@echo "hooks: core.hooksPath is .githooks"

build:
	go build ./cmd/trial

test:
	go test -timeout=3m ./...

race:
	go test -race -timeout=10m ./...

property:
	go test -timeout=3m ./internal/court -run '^TestGeneratedPrograms' -count=1

fuzz:
	go test -timeout=${FUZZ_TIMEOUT} ./internal/gregor -run '^$$' -fuzz '^FuzzParse$$' -fuzztime=${FUZZ_TIME} -parallel=4
	go test -timeout=${FUZZ_TIMEOUT} ./internal/counsel -run '^$$' -fuzz '^FuzzCounselReadMessage$$' -fuzztime=${FUZZ_TIME} -parallel=4
	go test -timeout=${FUZZ_TIMEOUT} ./internal/counsel -run '^$$' -fuzz '^FuzzCounselEnvelope$$' -fuzztime=${FUZZ_TIME} -parallel=4
	go test -timeout=${FUZZ_TIMEOUT} ./internal/advocate -run '^$$' -fuzz '^FuzzMCPIntegerID$$' -fuzztime=${FUZZ_TIME} -parallel=4
	go test -timeout=${FUZZ_TIMEOUT} ./internal/deposition -run '^$$' -fuzz '^FuzzDepositionParse$$' -fuzztime=${FUZZ_TIME} -parallel=4
	go test -timeout=${FUZZ_TIMEOUT} ./internal/court -run '^$$' -fuzz '^FuzzSumArithmetic$$' -fuzztime=${FUZZ_TIME} -parallel=4

coverage:
	go test -timeout=3m -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

fmt:
	go run golang.org/x/tools/cmd/goimports@${GOIMPORTS_VERSION} -w .

fmt-check:
	test -z "$$(gofmt -l .)"
	@files="$$(go run golang.org/x/tools/cmd/goimports@${GOIMPORTS_VERSION} -l .)" || exit $$?; \
		test -z "$$files" || { printf '%s\n' "$$files"; exit 1; }

vet:
	go vet ./...

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION} run ./...

tidy-check:
	go mod tidy -diff

examples:
	go run ./cmd/trial test examples

api-check:
	go doc -all ./canon | sed '$${/^$$/d;}' | diff -u docs/api.txt -

workflow-check:
	go run github.com/rhysd/actionlint/cmd/actionlint@${ACTIONLINT_VERSION} \
		-shellcheck= -pyflakes=

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION} ./...

licenses:
	go run github.com/google/go-licenses/v2@${GO_LICENSES_VERSION} check ./cmd/trial \
		--allowed_licenses=Apache-2.0,BSD-2-Clause,BSD-3-Clause,MIT

purego:
	CGO_ENABLED=0 go build ./...

arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./...

demo-generate:
	go run ./tools/demogen -write -root .

demo-check:
	go run ./tools/demogen -check -root .

.PHONY: doc-check gallery-generate gallery-check spec-check demos-check demos-record

# Recording is optional: it needs Bash, VHS, ttyd, FFmpeg, and Chromium.
# Validate ten tapes and nine local depositions before recording local demos.
# Recovery is separate: bash docs/demos/record.sh the-recovery (needs Kafka).
demos-check: build
	bash docs/demos/check.sh

demos-record: demos-check
	bash docs/demos/record.sh

doc-check:
	go run ./tools/doccheck -root .

gallery-generate:
	go run ./tools/examplegallery -write -root .

gallery-check:
	go run ./tools/examplegallery -check -root .

# OpenSpec is optional for Go builds. CI validates it in a separate Node job.
spec-check:
	OPENSPEC_TELEMETRY=0 ${NPX} --yes @fission-ai/openspec@${OPENSPEC_VERSION} validate --all --strict --no-interactive

verify:
	@test "$$(go env GOVERSION)" = "go${GO_VERSION}" || \
		{ echo "Go ${GO_VERSION} is required; found $$(go env GOVERSION)" >&2; exit 1; }
	${MAKE} fmt-check
	${MAKE} tidy-check
	${MAKE} vet
	${MAKE} test
	${MAKE} race
	${MAKE} fuzz
	${MAKE} lint
	${MAKE} api-check
	${MAKE} workflow-check
	${MAKE} licenses
	${MAKE} purego
	${MAKE} arm64
	${MAKE} demo-check
	${MAKE} doc-check
	${MAKE} gallery-check
	${MAKE} examples

# A bare `make` used to run the first target, which rewired core.hooksPath.
# Listing the targets is the safer default.
.DEFAULT_GOAL := help

.PHONY: help install clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo devel)

help:
	@echo "trial-lang development targets:"
	@echo
	@echo "  help           List development targets (the default)"
	@echo "  build          Compile ./cmd/trial into ./trial"
	@echo "  install        go install ./cmd/trial with the git version stamped in"
	@echo "  test           Test suite (Kafka tests require TRIAL_E2E_BROKER)"
	@echo "  race           Tests under the race detector"
	@echo "  property       Generated-program property tests"
	@echo "  fuzz           Language, deposition, protocols, and arithmetic fuzzing (FUZZ_TIME=${FUZZ_TIME})"
	@echo "  coverage       Statement coverage report"
	@echo "  examples       Run the brokerless example depositions"
	@echo "  fmt            Rewrite sources with goimports"
	@echo "  fmt-check      Check gofmt and goimports without rewriting files"
	@echo "  tidy-check     Check module tidiness without rewriting files"
	@echo "  vet            Go vet analyzers"
	@echo "  api-check      Check the public API snapshot"
	@echo "  workflow-check Validate GitHub Actions workflows"
	@echo "  licenses       Check dependency license compatibility"
	@echo "  purego         Build without CGO"
	@echo "  arm64          Cross-build for Linux ARM64"
	@echo "  demo-generate  Regenerate demo assets"
	@echo "  demo-check     Check generated demo assets"
	@echo "  doc-check      Check local Markdown link targets"
	@echo "  gallery-generate Regenerate previews from executed examples"
	@echo "  gallery-check  Check generated example previews"
	@echo "  demos-check    Validate ten VHS tapes and nine local depositions"
	@echo "  demos-record   Record nine local examples with VHS"
	@echo "  spec-check     Validate OpenSpec (requires Node.js and npm)"
	@echo "  lint           golangci-lint with the pinned version"
	@echo "  vuln           govulncheck"
	@echo "  verify         Required local checks; also run make vuln"
	@echo "  hooks          Point core.hooksPath at .githooks"
	@echo "  clean          Remove the local binary and coverage output"
	@echo
	@echo "Run 'make verify' before opening a pull request. See CONTRIBUTING.md."

install:
	go install -ldflags "-X main.version=$(VERSION)" ./cmd/trial

clean:
	rm -f trial trial.exe coverage.out
