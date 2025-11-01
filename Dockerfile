# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-debian12:debug-nonroot@sha256:e51574ad48c5766e7a05b6924bd763953004d48f5725dbd11ebf516d28c1639f
COPY bin/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
