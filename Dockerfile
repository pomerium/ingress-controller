# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-debian12:debug-nonroot@sha256:254c9629c6fad3f34af7ae24d76a74df0b4f436fc778d57d422721ad95ec31a2
COPY bin/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
