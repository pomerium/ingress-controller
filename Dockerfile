# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-nossl-debian12:debug-nonroot@sha256:83b8737817cda240f3f75e9acaa2b6fa547e2693e7e8dc4594c7ce98a0e5765e
COPY bin/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
