# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-debian12:debug-nonroot@sha256:c35d5c0b8b0ec585408d4edde5e66173b436c9cadc53bb0d23c6d95d528ace0c
COPY bin/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
