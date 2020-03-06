OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)
ALL_PLATFORM = linux/amd64,linux/arm/v7,linux/arm64

GOPATH ?= $(GOPATH)

REGISTRY ?= scaleway
IMAGE ?= scaleway-cloud-controller-manager
FULL_IMAGE ?= $(REGISTRY)/$(IMAGE)

TAG ?= $(shell git rev-parse HEAD)

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

.PHONY: build
compile:
	go build -v -o scaleway-cloud-controller-manager ./cmd/scaleway-cloud-controller-manager

.PHONY: docker-build
docker-build:
	@echo "Building scaleway-cloud-controller-manager for ${ARCH}"
	docker build . --platform=linux/$(ARCH) --build-arg ARCH=$(ARCH) --build-arg TAG=$(TAG) -f Dockerfile -t ${FULL_IMAGE}:${TAG}-$(ARCH)

.PHONY: docker-buildx-all
docker-buildx-all:
	@echo "Making release for tag $(TAG)"
	docker buildx build --platform=$(ALL_PLATFORM) --push -t $(FULL_IMAGE):$(TAG) .

## Release
.PHONY: release
release: docker-buildx-all
