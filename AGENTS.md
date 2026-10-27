# AGENTS.md

This repository is the Pomerium Kubernetes Ingress Controller.

## Dependency pins

- Pin GitHub Actions, container images, downloaded tools, and source archives to an immutable commit SHA, digest, or checksum. Keep a readable version beside each pin.
- GitHub Actions: write `uses: owner/repo@<40-character SHA> # vX.Y.Z`. After you add or change an action, run `pinact run` to write the SHA and the comment.
- Container images: write `<image>:<tag>@sha256:<digest>`. Use a version tag or a dated tag when the registry publishes one. Use a variant tag such as `latest` or `nonroot` only when no version tag points to the digest. Do not write a digest without a tag.
- Put the version in the image tag or in the `# vX.Y.Z` action comment. When Dependabot or an update script changes a pin, it changes the tag or the version comment together with the SHA or digest. It does not change other comments, so do not put a version or a date in a free-text comment next to an image digest.
- When a pin has no upstream release, write the upstream branch, date, and reason in a comment. Do not invent a version.
- The `Lint Pins` workflow fails a pull request that adds a bare action SHA, a version comment that does not match its SHA, or an image digest without a tag.
