# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-debian12:debug@sha256:02be0066ee51d3d8a77be84e7136df6b9946c46e114aa2ffc5f08027cc5840ff
COPY bin/manager /manager

ENTRYPOINT ["/manager"]
