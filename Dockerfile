# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-debian12:debug-nonroot@sha256:76acc040228aed628435f9951e0bee94f99645efabcdf362e94a8c70ba422f99
COPY bin/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
