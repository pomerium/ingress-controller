# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-debian12:debug-nonroot@sha256:d904990dc95bad1ee477aa15c3b40b95e96ea187fd75486957114e3a901de130
COPY bin/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
