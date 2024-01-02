# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-debian12:debug-nonroot@sha256:8548e3041a2cc583998c6a6beabf13ae93e6b006a5f6a6194966b4327ea741f5
COPY bin/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
