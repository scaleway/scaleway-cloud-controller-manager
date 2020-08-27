FROM golang:1.15.0-alpine as builder

RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates

WORKDIR /go/src/github.com/scaleway/scaleway-cloud-controller-manager

COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY cmd/ cmd/
COPY scaleway/ scaleway/

ARG TAG
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a -ldflags "-w -s -X github.com/scaleway/scaleway-cloud-controller-manager/scaleway.version=${TAG}" -o scaleway-cloud-controller-manager ./cmd/scaleway-cloud-controller-manager

FROM scratch
WORKDIR /
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /go/src/github.com/scaleway/scaleway-cloud-controller-manager/scaleway-cloud-controller-manager .
ENTRYPOINT ["/scaleway-cloud-controller-manager"]
