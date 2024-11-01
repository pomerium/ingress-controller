# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-debian12:debug-nonroot@sha256:2d0f47e5034542a240f52dd4007891c44e5fd0a13db33e1ae26ee83892d8a1e3
COPY bin/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
