# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-debian12:debug-nonroot@sha256:8d946e4103571ec0e2f471eac7c4859a7f169686d222f5c8b2d9d391d6d1e50c
COPY bin/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
