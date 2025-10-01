# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-debian12:debug-nonroot@sha256:61e3d3df3b7a1a811ca81778ded8286f60d403c127e1b7d2176c348e822e461a
COPY bin/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
