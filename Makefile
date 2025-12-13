.PHONY: all test test-unit test-integration test-race test-all lint vet fmt build clean \
        coverage verify-safepath security-scan check-deps tidy pre-release ci help

# Default target
all: build

# ============================================================================
# TESTING
# ============================================================================

# Run unit tests
test:
	go test -v ./...

# Run unit tests with coverage
test-unit:
	go test -v -coverprofile=coverage.out ./...

# Run integration tests
test-integration:
	go test -tags=integration -v -coverprofile=coverage.integration.out ./...

# Run tests with race detector
test-race:
	go test -v -race ./...

# Run all tests (unit + integration + race)
test-all: test-race test-integration

# ============================================================================
# CODE QUALITY
# ============================================================================

# Run go vet
vet:
	go vet ./...

# Run gofmt check (fails if files need formatting)
fmt-check:
	@echo "Checking code formatting..."
	@test -z "$$(gofmt -l .)" || (echo "Files need formatting:" && gofmt -l . && exit 1)

# Format code
fmt:
	gofmt -w .

# Run golangci-lint
lint:
	golangci-lint run --timeout=5m

# Verify safepath usage (no direct os file I/O)
verify-safepath:
	@./scripts/verify-safepath.sh

# ============================================================================
# SECURITY
# ============================================================================

# Run security scans
security-scan:
	@echo "Running gosec security scanner..."
	@which gosec > /dev/null || go install github.com/securego/gosec/v2/cmd/gosec@latest
	gosec -quiet ./...
	@echo ""
	@echo "Running govulncheck vulnerability scanner..."
	@which govulncheck > /dev/null || go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

# ============================================================================
# BUILD
# ============================================================================

# Build all packages
build:
	go build ./...

# Build with integration tag
build-integration:
	go build -tags=integration ./...

# ============================================================================
# DEPENDENCIES
# ============================================================================

# Tidy and verify dependencies
tidy:
	go mod tidy
	go mod verify

# Check for dependency updates
check-deps:
	@echo "Checking for dependency updates..."
	@go list -u -m all 2>/dev/null | grep -v "^go\|^toolchain" | head -20 || true

# ============================================================================
# COVERAGE
# ============================================================================

# Generate coverage reports
coverage: test-unit test-integration
	@echo ""
	@echo "=== Unit Test Coverage ==="
	@go tool cover -func=coverage.out | tail -1
	@echo ""
	@echo "=== Integration Test Coverage ==="
	@if [ -f coverage.integration.out ]; then \
		go tool cover -func=coverage.integration.out | tail -1; \
	fi

# Generate HTML coverage report
coverage-html: coverage
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# ============================================================================
# CI / RELEASE
# ============================================================================

# CI target - run by GitHub Actions
ci: tidy fmt-check vet lint verify-safepath test-race build
	@echo ""
	@echo "CI checks passed!"

# Pre-release checks - comprehensive validation before release
pre-release: clean tidy
	@echo "========================================"
	@echo "  GoExec Pre-Release Checks"
	@echo "========================================"
	@echo ""
	@echo "[1/8] Checking code formatting..."
	@$(MAKE) fmt-check
	@echo ""
	@echo "[2/8] Running go vet..."
	@$(MAKE) vet
	@echo ""
	@echo "[3/8] Running linters..."
	@$(MAKE) lint
	@echo ""
	@echo "[4/8] Verifying safepath usage..."
	@$(MAKE) verify-safepath
	@echo ""
	@echo "[5/8] Running security scans..."
	@$(MAKE) security-scan
	@echo ""
	@echo "[6/8] Running tests with race detector..."
	@$(MAKE) test-race
	@echo ""
	@echo "[7/8] Running integration tests..."
	@$(MAKE) test-integration
	@echo ""
	@echo "[8/8] Building all packages..."
	@$(MAKE) build
	@$(MAKE) build-integration
	@echo ""
	@echo "========================================"
	@echo "  All pre-release checks passed!"
	@echo "========================================"

# ============================================================================
# CLEANUP
# ============================================================================

# Clean generated files
clean:
	rm -f coverage.out coverage.integration.out coverage.html
	rm -f gosec-report.json gosec-report.sarif
	go clean -cache -testcache

# ============================================================================
# HELP
# ============================================================================

help:
	@echo "GoExec Makefile targets:"
	@echo ""
	@echo "  Testing:"
	@echo "    test              Run unit tests"
	@echo "    test-unit         Run unit tests with coverage"
	@echo "    test-integration  Run integration tests"
	@echo "    test-race         Run tests with race detector"
	@echo "    test-all          Run all tests"
	@echo ""
	@echo "  Code Quality:"
	@echo "    vet               Run go vet"
	@echo "    fmt               Format code with gofmt"
	@echo "    fmt-check         Check code formatting"
	@echo "    lint              Run golangci-lint"
	@echo "    verify-safepath   Verify no direct os file I/O"
	@echo ""
	@echo "  Security:"
	@echo "    security-scan     Run gosec and govulncheck"
	@echo ""
	@echo "  Build:"
	@echo "    build             Build all packages"
	@echo "    build-integration Build with integration tag"
	@echo ""
	@echo "  Dependencies:"
	@echo "    tidy              Tidy and verify go.mod"
	@echo "    check-deps        Check for dependency updates"
	@echo ""
	@echo "  Coverage:"
	@echo "    coverage          Generate coverage reports"
	@echo "    coverage-html     Generate HTML coverage report"
	@echo ""
	@echo "  CI/Release:"
	@echo "    ci                Run CI checks"
	@echo "    pre-release       Run all pre-release checks"
	@echo ""
	@echo "  Cleanup:"
	@echo "    clean             Remove generated files"
