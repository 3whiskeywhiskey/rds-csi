# RDS CSI Driver Makefile

# Image configuration
REGISTRY ?= ghcr.io/whiskey
IMAGE_NAME ?= rds-csi-driver
IMAGE_TAG ?= dev
IMAGE = $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

# Build configuration
GOARCH ?= amd64
GOOS ?= linux
CGO_ENABLED ?= 0

# Binary output
BINARY_NAME = rds-csi-plugin
BUILD_DIR = bin
DOCKERFILE = Dockerfile

# Go build flags
LDFLAGS ?= -s -w
GOFLAGS ?= -v

# Git versioning
GIT_COMMIT := $(shell git rev-parse --short HEAD)
GIT_TAG := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
VERSION_PKG = git.srvlab.io/whiskey/rds-csi-driver/pkg/driver

VERSION_LDFLAGS := -X $(VERSION_PKG).version=$(GIT_TAG) \
                    -X $(VERSION_PKG).gitCommit=$(GIT_COMMIT) \
                    -X $(VERSION_PKG).buildDate=$(BUILD_DATE)

# Targets
.PHONY: all
all: build

.PHONY: help
help:
	@echo "RDS CSI Driver - Build Targets:"
	@echo ""
	@echo "  make build         - Build binary for Linux/amd64"
	@echo "  make docker        - Build Docker image"
	@echo "  make docker-push   - Push Docker image to registry"
	@echo "  make test          - Run unit tests"
	@echo "  make test-coverage - Run tests with coverage report"
	@echo "  make sanity        - Run CSI sanity tests"
	@echo "  make lint          - Run linters (golangci-lint)"
	@echo "  make fmt           - Format Go code"
	@echo "  make clean         - Remove build artifacts"
	@echo "  make mod-tidy      - Tidy Go modules"
	@echo "  make verify        - Run all verification checks"
	@echo ""

.PHONY: build
build: mod-tidy
	@echo "Building $(BINARY_NAME) for $(GOOS)/$(GOARCH)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build \
		-ldflags "$(LDFLAGS) $(VERSION_LDFLAGS)" \
		$(GOFLAGS) \
		-o $(BUILD_DIR)/$(BINARY_NAME) \
		./cmd/rds-csi-plugin
	@echo "Binary created: $(BUILD_DIR)/$(BINARY_NAME)"

.PHONY: build-local
build-local:
	@echo "Building $(BINARY_NAME) for local OS..."
	@mkdir -p $(BUILD_DIR)
	go build \
		-ldflags "$(LDFLAGS) $(VERSION_LDFLAGS)" \
		$(GOFLAGS) \
		-o $(BUILD_DIR)/$(BINARY_NAME) \
		./cmd/rds-csi-plugin
	@echo "Binary created: $(BUILD_DIR)/$(BINARY_NAME)"

.PHONY: docker
docker:
	@echo "Building Docker image: $(IMAGE)"
	docker build \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg GIT_TAG=$(GIT_TAG) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMAGE) \
		-f $(DOCKERFILE) \
		.
	@echo "Image built: $(IMAGE)"

.PHONY: docker-push
docker-push: docker
	@echo "Pushing Docker image: $(IMAGE)"
	docker push $(IMAGE)

.PHONY: docker-multiarch
docker-multiarch:
	@echo "Building multi-arch Docker images: $(IMAGE)"
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg GIT_TAG=$(GIT_TAG) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMAGE) \
		-f $(DOCKERFILE) \
		--push \
		.

.PHONY: test
test:
	@echo "Running unit tests..."
	go test -v -race -timeout 5m ./pkg/...

.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./pkg/...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

.PHONY: sanity
sanity:
	@echo "Running CSI sanity tests..."
	@echo "Requires csi-sanity to be installed:"
	@echo "  go install github.com/kubernetes-csi/csi-test/cmd/csi-sanity@latest"
	@if ! command -v csi-sanity &> /dev/null; then \
		echo "ERROR: csi-sanity not found. Install it first."; \
		exit 1; \
	fi
	@echo "Starting CSI driver..."
	@./$(BUILD_DIR)/$(BINARY_NAME) --endpoint=unix:///tmp/csi.sock --node-id=test-node &
	@sleep 2
	@echo "Running sanity tests..."
	csi-sanity --csi.endpoint=/tmp/csi.sock
	@killall $(BINARY_NAME) || true
	@rm -f /tmp/csi.sock

.PHONY: lint
lint:
	@echo "Running linters..."
	@if ! command -v golangci-lint &> /dev/null; then \
		echo "Installing golangci-lint..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	fi
	golangci-lint run --timeout 5m

.PHONY: fmt
fmt:
	@echo "Formatting Go code..."
	go fmt ./...
	@echo "Running goimports..."
	@if ! command -v goimports &> /dev/null; then \
		echo "Installing goimports..."; \
		go install golang.org/x/tools/cmd/goimports@latest; \
	fi
	goimports -w -local git.srvlab.io/whiskey/rds-csi-driver .

.PHONY: vet
vet:
	@echo "Running go vet..."
	go vet ./...

.PHONY: verify
verify: fmt vet lint test
	@echo "All verification checks passed!"

.PHONY: mod-tidy
mod-tidy:
	@echo "Tidying Go modules..."
	go mod tidy

.PHONY: mod-vendor
mod-vendor: mod-tidy
	@echo "Vendoring dependencies..."
	go mod vendor

.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	rm -rf vendor/
	@echo "Clean complete."

.PHONY: install-tools
install-tools:
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/kubernetes-csi/csi-test/cmd/csi-sanity@latest
	@echo "Tools installed."

# Deployment targets
.PHONY: deploy
deploy:
	@echo "Deploying to Kubernetes..."
	kubectl apply -f deploy/kubernetes/

.PHONY: deploy-helm
deploy-helm:
	@echo "Installing via Helm..."
	helm upgrade --install rds-csi \
		deploy/helm/rds-csi-driver \
		--namespace kube-system \
		--create-namespace

.PHONY: undeploy
undeploy:
	@echo "Removing from Kubernetes..."
	kubectl delete -f deploy/kubernetes/ --ignore-not-found=true

.PHONY: logs-controller
logs-controller:
	kubectl logs -n kube-system -l app=rds-csi-controller -c rds-csi-plugin -f

.PHONY: logs-node
logs-node:
	kubectl logs -n kube-system -l app=rds-csi-node -c rds-csi-plugin -f

# Version info
.PHONY: version
version:
	@echo "Version:     $(GIT_TAG)"
	@echo "Git Commit:  $(GIT_COMMIT)"
	@echo "Build Date:  $(BUILD_DATE)"

# CI targets
.PHONY: ci-build
ci-build: mod-tidy lint test build

.PHONY: ci-image
ci-image: docker

.PHONY: ci-release
ci-release: verify docker-multiarch
	@echo "Release build complete"
