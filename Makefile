OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)
ARCHS ?= amd64 arm64

BUILD_DATE ?= $(shell date -Is)

GOPATH ?= $(GOPATH)

REGISTRY ?= scaleway
IMAGE ?= scaleway-cloud-controller-manager
FULL_IMAGE ?= $(REGISTRY)/$(IMAGE)

TAG ?= $(shell git rev-parse HEAD)
IMAGE_TAG ?= $(shell git rev-parse HEAD)
SOURCE_TAG ?= $(IMAGE_TAG)
COMMIT_SHA ?= $(shell git rev-parse HEAD)

DOCKER_CLI_EXPERIMENTAL ?= enabled

.PHONY: default
default: test compile

.PHONY: clean
clean:
	go clean -i -x ./...

.PHONY: test
test:
	go test -timeout=1m -v -race -short ./...

.PHONY: fmt
fmt:
	find . -type f -name "*.go" | grep -v "./vendor/*" | xargs gofmt -s -w -l

.PHONY: compile
compile:
	go build -v -o scaleway-cloud-controller-manager ./cmd/scaleway-cloud-controller-manager

.PHONY: docker-build
docker-build:
	@echo "Building scaleway-cloud-controller-manager for ${ARCH}"
	docker build . --platform=linux/$(ARCH) --build-arg ARCH=$(ARCH) --build-arg TAG=$(TAG) --build-arg COMMIT_SHA=$(COMMIT_SHA) --build-arg BUILD_DATE=$(BUILD_DATE) -f Dockerfile -t ${FULL_IMAGE}:${IMAGE_TAG}-$(ARCH)

.PHONY: docker-push-arch
docker-push-arch:
	@echo "Building and pushing scaleway-cloud-controller-manager for $(ARCH)"
	docker buildx build . --platform=linux/$(ARCH) --build-arg TAG=$(TAG) --build-arg COMMIT_SHA=$(COMMIT_SHA) --build-arg BUILD_DATE=$(BUILD_DATE) --push -t $(FULL_IMAGE):$(IMAGE_TAG)-$(ARCH)

.PHONY: docker-manifest
docker-manifest:
	@echo "Creating manifest $(FULL_IMAGE):$(IMAGE_TAG) from $(foreach arch,$(ARCHS),$(FULL_IMAGE):$(SOURCE_TAG)-$(arch))"
	docker buildx imagetools create -t $(FULL_IMAGE):$(IMAGE_TAG) $(foreach arch,$(ARCHS),$(FULL_IMAGE):$(SOURCE_TAG)-$(arch))

.PHONY: docker-push-all
docker-push-all:
	@for arch in $(ARCHS); do $(MAKE) docker-push-arch ARCH=$$arch; done
	$(MAKE) docker-manifest

## Release
.PHONY: release
release: docker-push-all
