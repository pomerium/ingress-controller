# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-debian12:debug-nonroot@sha256:530b4510719990e330caf2105ec8071328b4f92329abd32da8ccf96d2170eaf1
COPY bin/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
