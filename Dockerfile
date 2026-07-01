# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-debian12:debug-nonroot@sha256:ddd86b705dac25b3cc5f9d580018c6397c6b02ae5c2fa58ae95409c71e73cc3b
COPY bin/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
