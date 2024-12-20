# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-debian12:debug-nonroot@sha256:d88b20e321d3f81d9f712bff98caffef5d4cd2066bbda3e18c1c81d3441d4d4c
COPY bin/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
