# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-debian12:debug-nonroot@sha256:8c310805a18142025a807032583e794b63f9d8ce6cc3018edfc9827c909109cd
COPY bin/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
