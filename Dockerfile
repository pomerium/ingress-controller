# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/base-nossl-debian13:debug-nonroot@sha256:37b8b2d30e81bf028eae7c1e8c65d775d46b2a21a2b76cde6bf8bf59ef0e3289
COPY licenses/POMERIUM-COMMUNITY-LICENSE-1.0.txt /pomerium/LICENSE
COPY bin/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
