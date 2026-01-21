#!/usr/bin/env bash

set -euo pipefail
IFS=$'\n\t'

# Modifies the Pomerium deployment and the databroker statefulset
# then cuts a new release with the same version as the latest Pomerium release

# requires the gh tool
# requires jq

# Ensure we're on main
if [[ "$(git branch --show-current)" != "main" ]]; then
  echo "Error: Not on main branch" >&2
  exit 1
fi

##Ensure there are no changes
if [[ -n "$(git status --porcelain)" ]]; then
  echo "Error: Working directory has uncommitted changes" >&2
  exit 1
fi

latest=$(gh release view --repo pomerium/pomerium --json tagName --jq '.tagName')
if ! [[ "$latest" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  printf "Got latest pomerium version %s. Invalid semver\n" "${latest}"
  exit 1
fi

branch_name=$(echo "${latest}" | tr -d 'v' | tr '.' '-')

printf "About to modify images and create a new release for ingress-controller@%s\n" "${latest}"
select yn in "Yes" "No"; do
    case $yn in
        Yes ) make install; break;;
        No ) exit;;
    esac
done


git checkout -b "${branch_name}"

# Update the pomerium version
yq -i '(.spec.template.spec.containers[] | select(.name == "pomerium") | .image) = "pomerium/ingress-controller:" + env(latest)' config/pomerium/deployment/image.yaml
yq -i '(.spec.template.spec.containers[] | select(.name == "pomerium") | .imagePullPolicy) = "IfNotPresent"' config/pomerium/deployment/image.yaml

yq -i '(.spec.template.spec.containers[] | select(.name == "pomerium") | .image) = "pomerium/ingress-controller:" + env(latest)' config/clustered-databroker/statefulset/image.yaml
yq -i '(.spec.template.spec.containers[] | select(.name == "pomerium") | .imagePullPolicy) = "IfNotPresent"' config/clustered-databroker/statefulset/image.yaml

git add config/pomerium/deployment/image.yaml
git add config/clustered-databroker/statefulset/image.yaml

make lint
make deployment

git commit -m "Customize ingress controller ${latest}"
git push origin "${branch_name}"

gh release create "$latest" --target "$branch_name" --title "$latest" --prerelease
