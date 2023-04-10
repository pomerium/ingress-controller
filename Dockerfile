# Build the manager binary
# make sure to run `make clean` if building locally

FROM golang:1.20.2@sha256:1724dc3128e2e63f0bc3e055fe4fa478d67f6da4bd95c0e69690f6435f658804 as go-modules

WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum
COPY scripts scripts
COPY Makefile Makefile

RUN mkdir -p pomerium/envoy/bin
RUN make envoy
RUN go mod download

COPY Makefile ./Makefile

# download ui dependencies from core module
RUN mkdir -p internal
RUN make internal/ui

FROM node:16@sha256:b9fe422fdf0d51f616d25aa6ccc0d900eb25ca08bd78d79e369c480b4584c3a8 as ui
WORKDIR /workspace

COPY --from=go-modules /workspace/internal/ui ./
RUN yarn install
RUN yarn build

FROM go-modules as go-builder
WORKDIR /workspace

# Copy the go source
COPY . .

COPY --from=ui /workspace/dist ./internal/ui/dist

# Build
RUN CGO_ENABLED=0 make build-go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base:debug-nonroot@sha256:344ef238cd8c6027777153fd738189b451dd789cb8023dc954c2e8bdd322700f
WORKDIR /
COPY --from=go-builder /workspace/bin/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
