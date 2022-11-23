# Build the manager binary
# make sure to run `make clean` if building locally

FROM golang:1.19.3@sha256:dc76ef03e54c34a00dcdca81e55c242d24b34d231637776c4bb5c1a8e8514253 as go-modules
WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY scripts scripts
COPY Makefile Makefile
RUN mkdir -p pomerium/envoy/bin \
    && make envoy

# download ui dependencies from core module
RUN mkdir -p internal \
    && make internal/ui

FROM node:16@sha256:b9fe422fdf0d51f616d25aa6ccc0d900eb25ca08bd78d79e369c480b4584c3a8 as ui
WORKDIR /workspace

COPY --from=go-modules /workspace/internal/ui ./
RUN yarn install --network-timeout 120000 \
    && yarn build

FROM go-modules as go-builder
WORKDIR /workspace

# Copy the go source
COPY . .
COPY --from=ui /workspace/dist ./internal/ui/dist

# Build
RUN CGO_ENABLED=0 make build-go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base:debug-nonroot@sha256:90d0605800a57e3caec404f84b79751958e0132596aacfbfdc94eb1933835a2b
WORKDIR /
COPY --from=go-builder /workspace/bin/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
