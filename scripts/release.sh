#!/usr/bin/env bash

set -euo pipefail
IFS=$'\n\t'

# Modifies the Pomerium deployment and the databroker statefulset
# then cuts a new release with the provided semver.

# requires the gh tool
# requires jq

if [ $# -lt 1 ]; then
  echo "Usage: ./scripts/release.sh <vX.Y.Z> where vX.Y.Z is the semver of the ingress-controller release."
  exit 1
fi

new_version="${1:-blank}"
if ! [[ "$new_version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  printf "Got desired version %s. Invalid semver\n" "${new_version}"
  exit 1
fi

ensure_branch () {
  desired=$1
  if [[ "$(git branch --show-current)" != "${desired}" ]]; then
    echo "Error: Not on ${desired} branch" >&2
    exit 1
  fi
}

ensure_no_changes() {
  if [[ -n "$(git status --porcelain)" ]]; then
    echo "Error: Working directory has uncommitted changes" >&2
    exit 1
  fi
}

# Ensure we're on main
ensure_branch "main"

# Ensure there are no changes
ensure_no_changes

branch_name=$(echo "${new_version}" | tr -d 'v' | tr '.' '-')

printf "About to modify images and create a new release for ingress-controller@%s\n" "${new_version}"
select yn in "Yes" "No"; do
    case $yn in
        Yes ) make install; break;;
        No ) exit;;
    esac
done


git checkout -b "${branch_name}"

# Update the pomerium version
yq -i '(.spec.template.spec.containers[] | select(.name == "pomerium") | .image) = "pomerium/ingress-controller:" + env(new_version)' config/pomerium/deployment/image.yaml
yq -i '(.spec.template.spec.containers[] | select(.name == "pomerium") | .imagePullPolicy) = "IfNotPresent"' config/pomerium/deployment/image.yaml

yq -i '(.spec.template.spec.containers[] | select(.name == "pomerium") | .image) = "pomerium/ingress-controller:" + env(new_version)' config/clustered-databroker/statefulset/image.yaml
yq -i '(.spec.template.spec.containers[] | select(.name == "pomerium") | .imagePullPolicy) = "IfNotPresent"' config/clustered-databroker/statefulset/image.yaml

git add config/pomerium/deployment/image.yaml
git add config/clustered-databroker/statefulset/image.yaml

make lint
make deployment

git add deployment.yaml

git commit -m "Customize ingress controller ${new_version}"

##Ensure there are no changes again
ensure_no_changes

git push origin "${branch_name}"

gh release create "$new_version" --target "$branch_name" --title "$new_version" --prerelease
