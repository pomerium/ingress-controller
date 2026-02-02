# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-debian12:debug-nonroot@sha256:aa786ea4fd24a4fdea4ffdfe5a3540a6ea39ac90d93c4bd4f589d7378706b3fe
COPY bin/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
