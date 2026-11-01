# checks that image tag matches the argument

set -e
want=$1
if [[ -z "${want}" ]]; then
    echo "Usage: $0 <tag>"
    exit 1
fi

for file in config/pomerium/deployment/image.yaml config/gen_secrets/job.yaml; do
    img=$(yq eval '.spec.template.spec.containers[0].image' "${file}")
    tag=${img#pomerium/ingress-controller:}
    if [[ "${tag}" != "${want}" ]]; then
        echo "Image tag mismatch in ${file}: ${tag} != ${want}"
        exit 1
    fi
done
