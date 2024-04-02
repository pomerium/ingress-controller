# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-debian12:debug-nonroot@sha256:08baf3b9b25ef61b205195747e8d8b746f10317cc37a0250f9ca66312be8bd1d
COPY bin/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
