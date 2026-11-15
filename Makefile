SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
GO ?= go
PYTHON ?= python3
export CGO_ENABLED := 1
COVERPKG := ./internal/service,./internal/repository,./internal/handler

.PHONY: build run test test-unit test-integration test-security coverage test-e2e test-load pdf fmt vet environment clean branches
build:
	mkdir -p bin
	$(GO) build -trimpath -buildvcs=false -o bin/tasks ./cmd/tasks
run: build
	./bin/tasks
test: test-unit test-integration coverage
test-unit:
	mkdir -p reports
	$(GO) test -race -count=1 -json -run '^TestU' ./internal/service | tee reports/unit.jsonl | $(PYTHON) tools/go_test_output.py | tee reports/unit.log
test-integration:
	mkdir -p reports
	$(GO) test -race -count=1 -json -run '^TestI' ./internal/repository ./internal/handler | tee reports/integration.jsonl | $(PYTHON) tools/go_test_output.py | tee reports/integration.log
test-security:
	mkdir -p reports
	$(GO) test -race -count=1 -json -run '^TestS03' ./internal/handler | tee reports/security-sql.jsonl | $(PYTHON) tools/go_test_output.py | tee reports/security-sql.log
coverage:
	mkdir -p reports
	$(GO) test -race -count=1 -covermode=atomic -coverpkg=$(COVERPKG) -coverprofile=reports/coverage.out ./internal/...
	$(GO) tool cover -func=reports/coverage.out | tee reports/coverage-functions.txt
	$(GO) tool cover -html=reports/coverage.out -o reports/coverage.html
	$(PYTHON) tools/check_coverage.py reports/coverage.out
test-e2e: build
	cd e2e && npm ci && npm run typecheck && npm test
test-load: build
	$(PYTHON) tools/run_load.py
pdf:
	mkdir -p reports/pdf
	pdflatex -interaction=nonstopmode -halt-on-error -output-directory=reports/pdf docs/task_test_plan.tex
	pdflatex -interaction=nonstopmode -halt-on-error -output-directory=reports/pdf docs/task_test_plan.tex
fmt:
	$(GO) fmt ./...
vet:
	$(GO) vet ./...
branches:
	$(PYTHON) tools/branch_coverage.py
environment:
	$(PYTHON) tools/environment.py
clean:
	rm -rf bin reports e2e/test-results e2e/playwright-report
