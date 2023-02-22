# Build the manager binary
# make sure to run `make clean` if building locally

FROM golang:1.20.1@sha256:2edf6aab2d57644f3fe7407132a0d1770846867465a39c2083770cf62734b05d as go-modules

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
FROM gcr.io/distroless/base:debug-nonroot@sha256:b91cfa6bb6df121257046b06f278c49d9ab66238804b28c346908ed172ebd593
WORKDIR /
COPY --from=go-builder /workspace/bin/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
