# checks that image tag matches the argument

set -e
want=$1
if [[ -z "${want}" ]]; then
    echo "Usage: $0 <tag>"
    exit 1
fi

img=$(yq eval '.spec.template.spec.containers[0].image' config/pomerium/deployment/image.yaml)
tag=${img#pomerium/ingress-controller:}
if [[ "${tag}" != "${want}" ]]; then
    echo "Image tag mismatch: ${tag} != ${want}"
    exit 1
fi
