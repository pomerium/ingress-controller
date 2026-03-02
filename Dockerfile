# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-debian12:debug-nonroot@sha256:8ad538089479a821c32d9a8169419e39a1cf4de9adfb0d7e652d7fbeea456a6b
COPY bin/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
