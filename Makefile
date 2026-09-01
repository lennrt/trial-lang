GO_VERSION := 1.27.0
GOLANGCI_LINT_VERSION := v2.13.2
GOIMPORTS_VERSION := v0.49.0
GOVULNCHECK_VERSION := v1.1.4
GO_LICENSES_VERSION := v2.0.1
ACTIONLINT_VERSION := v1.7.12

.PHONY: hooks build test race property fmt fmt-check vet lint tidy-check examples api-check workflow-check vuln licenses purego arm64 demo-generate demo-check verify

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

verify:
	@test "$$(go env GOVERSION)" = "go${GO_VERSION}" || \
		{ echo "Go ${GO_VERSION} is required; found $$(go env GOVERSION)" >&2; exit 1; }
	${MAKE} fmt-check
	${MAKE} tidy-check
	${MAKE} vet
	${MAKE} test
	${MAKE} race
	${MAKE} lint
	${MAKE} api-check
	${MAKE} workflow-check
	${MAKE} licenses
	${MAKE} purego
	${MAKE} arm64
	${MAKE} demo-check
	${MAKE} examples
