KUBEENV_GOARCH=$(shell go env GOARCH)

CRD_BASE=github.com/pomerium/ingress-controller/apis/

# Image URL to use all building/pushing image targets
IMG?=ingress-controller:latest
CRD_OPTIONS?=
ENVTEST_K8S_VERSION?=$(shell go list -f '{{.Module.Version}}' k8s.io/api | sed -E 's/v0/1/; s/([0-9]+\.[0-9]+)\.[0-9]+/\1.x/')
SETUP_ENVTEST=go run sigs.k8s.io/controller-runtime/tools/setup-envtest@v0.0.0-20251010203701-b9bccfd41914
CONTROLLER_GEN=go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.18.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Pomerium core requires a special tag set to indicate
# the embedded resources would be supplied externally
GOTAGS = -tags embed_pomerium

GOLDFLAGS = -X github.com/pomerium/pomerium/internal/version.Version=$(shell go list -f '{{.Module.Version}}' github.com/pomerium/pomerium) \
	-X github.com/pomerium/pomerium/internal/version.BuildMeta=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ") \
	-X github.com/pomerium/pomerium/internal/version.ProjectName=pomerium-ingress-controller \
	-X github.com/pomerium/pomerium/internal/version.ProjectURL=https://www.pomerium.io

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build
	@echo "==> $@"

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: generated
generated: config/crd/bases/ingress.pomerium.io_pomerium.yaml apis/ingress/v1/zz_generated.deepcopy.go config/crd/bases/gateway.pomerium.io_policyfilters.yaml apis/gateway/v1alpha1/zz_generated.deepcopy.go
	@echo "==> $@"

apis/ingress/v1/zz_generated.deepcopy.go: apis/ingress/v1/pomerium_types.go
	@echo "==> $@"
	@$(CONTROLLER_GEN) object paths=$(CRD_BASE)/ingress/v1 output:dir=apis/ingress/v1

config/crd/bases/ingress.pomerium.io_pomerium.yaml: apis/ingress/v1/pomerium_types.go
	@echo "==> $@"
	@$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role crd paths=$(CRD_BASE)/ingress/v1 output:crd:artifacts:config=config/crd/bases

apis/gateway/v1alpha1/zz_generated.deepcopy.go: apis/gateway/v1alpha1/filter_types.go
	@echo "==> $@"
	@$(CONTROLLER_GEN) object paths=$(CRD_BASE)/gateway/v1alpha1 output:dir=apis/gateway/v1alpha1

config/crd/bases/gateway.pomerium.io_policyfilters.yaml: apis/gateway/v1alpha1/filter_types.go
	@echo "==> $@"
	@$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role crd paths=$(CRD_BASE)/gateway/v1alpha1 output:crd:artifacts:config=config/crd/bases

.PHONY: test
test: envoy generated pomerium-ui
	@echo "==> $@, k8s=$(ENVTEST_K8S_VERSION)"
	@KUBEBUILDER_ASSETS="$(shell $(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path --arch=$(KUBEENV_GOARCH))" go test $(GOTAGS) ./...

.PHONY: lint
lint: envoy pomerium-ui
	@echo "==> $@"
	@VERSION=$$(go run github.com/mikefarah/yq/v4@v4.34.1 '.jobs.lint.steps[] | select(.uses == "golangci/golangci-lint-action*") | .with.version' .github/workflows/lint.yml) && \
		go run github.com/golangci/golangci-lint/cmd/golangci-lint@$$VERSION run --fix ./...

##@ Build
.PHONY: build
build: generated pomerium-ui build-go ## Build manager binary.
	@echo "==> $@"


# called from github actions to build multi-arch images outside of docker
.PHONY: build-ci
build-ci: envoy-ci pomerium-ui
	@GOOS=linux GOARCH=amd64 go build $(GOTAGS) --ldflags="$(GOLDFLAGS)" -o bin/manager-linux-amd64 main.go
	@GOOS=linux GOARCH=arm64 go build $(GOTAGS) --ldflags="$(GOLDFLAGS)" -o bin/manager-linux-arm64 main.go

##@ Build
.PHONY: build-go
build-go: envoy
	@echo "==> $@"
	@go build $(GOTAGS) --ldflags="$(GOLDFLAGS)" -o bin/manager main.go

.PHONY: envoy-ci
envoy-ci: envoy
	@echo "==> $@"

.PHONY: envoy
envoy:
	@echo "==> $@"
	mkdir -p ./pomerium/envoy/bin && cd ./pomerium/envoy/bin && env -u GOOS go run github.com/pomerium/pomerium/pkg/envoy/get-envoy

UI_DIR = $(shell go list -f {{.Dir}} github.com/pomerium/pomerium/ui)
internal/ui:
	@echo "==> $@"
	@cp -rf $(UI_DIR) ./internal
	@chmod u+w internal/ui internal/ui/dist

internal/ui/package.json: internal/ui
	@echo "==> $@"
	@cd internal/ui && npm ci

.PHONY: pomerium-ui
pomerium-ui: internal/ui/dist/index.js
internal/ui/dist/index.js: internal/ui/package.json
	@echo "==> $@"
	@cd internal/ui && npm run build

# run the controller locally (i.e. with docker-desktop)
# that assumes that the CRDs and ingress class are already installed
.PHONY: run
run: generated
	@echo "==> $@"
	@go run $(GOTAGS) ./main.go all-in-one --pomerium-config global --update-status-from-service=pomerium/pomerium-proxy

.PHONY: docker-build
docker-build: build test ## Build docker image with the manager.
	@echo "==> $@"
	@docker build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	@echo "==> $@"
	@docker push ${IMG}

.PHONE: snapshot
snapshot:
	@echo "==> $@"
	@goreleaser release --snapshot --rm-dist

##@ Deployment

.PHONY: install
install: generated kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	@echo "==> $@"
	@$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: generated kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	@echo "==> $@"
	@$(KUSTOMIZE) build config/crd | kubectl delete -f -

.PHONY: deploy
deploy: generated kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	@echo "==> $@"
	@cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	@$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	@echo "==> $@"
	@$(KUSTOMIZE) build config/default | kubectl delete -f -

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	@echo "==> $@"
	@mkdir -p $(LOCALBIN)

.PHONY: clean
clean:
	@echo "==> $@"
	@chmod -Rf u+w ./bin || true
	@rm -rf pomerium/envoy/bin/*
	@rm -rf $(LOCALBIN)
	@rm -rf testbin
	@chmod -Rf u+w internal/ui || true
	@rm -rf internal/ui

KUSTOMIZE ?= $(LOCALBIN)/kustomize


KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE)  ## Download kustomize locally if necessary.
	@echo "==> $@"

$(KUSTOMIZE): $(LOCALBIN)
	@echo "==> $@"
	@rm -rf $(KUSTOMIZE)
	@curl -s $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN)

.PHONY: deployment
deployment: kustomize
	@echo "==> $@"
	@$(KUSTOMIZE) build config/default > deployment.yaml

.PHONY: docs
docs: generated
	@echo "==> $@"
	@go run docs/cmd/main.go > reference.md

#
# --- internal development targets
#
.PHONY: dev-install
dev-install:
	@echo "==> $@"
	@echo "deleting pods..."
	#@kubectl delete --force --selector app.kubernetes.io/name=pomerium pods || true
	@kubectl delete deployment/pomerium -n pomerium --wait || true
	@$(KUSTOMIZE) build config/dev/local --load-restrictor LoadRestrictionsNone | kubectl apply --filename -

.PHONY: dev-logs
dev-logs:
	@stern -n pomerium --selector app.kubernetes.io/name=pomerium

.PHONY: dev-gen-secrets
dev-gen-secrets:
	@echo "==> $@"
	@$(KUSTOMIZE) build config/dev/gen_secrets | kubectl apply --filename -

.PHONY: dev-build
dev-build:
	@echo "==> $@"
	@make -e GOOS=linux envoy
	@GOOS=linux GOARCH=arm64 go build $(GOTAGS) -o bin/manager-linux-arm64 main.go

.PHONY: dev-clean
dev-clean:
	@echo "==> $@"
	@kubectl delete ns/pomerium --wait || true
