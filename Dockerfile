# syntax=docker/dockerfile:1

FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder
WORKDIR /opt/app-root/src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
	-o /tmp/abi-minimal-initrd-inject ./cmd/abi-minimal-initrd-inject

FROM registry.access.redhat.com/ubi9/ubi-minimal:9.7

RUN microdnf -y install --nodocs xorriso \
	&& microdnf clean all \
	&& rm -rf /var/cache/yum /var/lib/dnf/history.*

COPY --from=builder /tmp/abi-minimal-initrd-inject /usr/local/bin/

WORKDIR /work

ENTRYPOINT ["/usr/local/bin/abi-minimal-initrd-inject"]
