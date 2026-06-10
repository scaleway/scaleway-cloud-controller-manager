# Development setup

The CCM has a simple build system based on make.

## Requirements

- Go (1.26+)
- make
- docker

## Unit testing

Run the following command to run the unit tests.

```
make test
```

## Building

Build the `scaleway-cloud-controller-manager` binary:

```
make compile
```

Format the code with `gofmt`:

```
make fmt
```

Remove build artifacts:

```
make clean
```

## Docker images

Build a local docker image for your current architecture. Set `REGISTRY` to your own
registry/namespace to avoid building into the `scaleway` namespace:

```
REGISTRY=[registry] make docker-build
```

Build and push a multi-arch image (amd64 and arm64) and create the manifest:

```
REGISTRY=[registry] make docker-push-all
```

`make release` is an alias for `make docker-push-all`.
