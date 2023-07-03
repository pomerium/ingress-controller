# Build the manager binary
# make sure to run `make clean` if building locally

FROM golang:1.20.5@sha256:344193a70dc3588452ea39b4a1e465a8d3c91f788ae053f7ee168cebf18e0a50 as go-modules

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

FROM node:lts-buster@sha256:3801c22eb9d286a58537c24e764affc9380bfee98aec4cb312ec488a0529ff3d as ui
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
FROM gcr.io/distroless/base:debug-nonroot@sha256:de8fb012fc630b7cdea6861442a0185213b574c71e246ddc97e9eb1d047048e7
WORKDIR /
COPY --from=go-builder /workspace/bin/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
