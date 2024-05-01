# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-debian12:debug-nonroot@sha256:8aa916570dcb9fdc8ffd1324a605ae2987cc4eaff3c927f454f6f2deef5c5184
COPY bin/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
