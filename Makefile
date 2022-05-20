# Only x64 is supported for most tools

export GOARCH = amd64

# Image URL to use all building/pushing image targets
IMG ?= ingress-controller:latest
CRD_OPTIONS ?=
ENVTEST_K8S_VERSION = 1.23

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

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

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	@echo "==> $@"
	@$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	@echo "==> $@"
	@$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
	@go generate ./...

.PHONY: fmt
fmt: ## Run go fmt against code.
	@echo "==> $@"
	@go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	@echo "==> $@"
	@go vet ./...

.PHONY: test
test: envoy manifests generate fmt vet envtest ## Run tests.
	@echo "==> $@"
	@KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path --arch=$(GOARCH))" go test ./... -coverprofile cover.out

.PHONY: lint
lint: envoy ## Verifies `golint` passes.
	@echo "==> $@"
	@go run github.com/golangci/golangci-lint/cmd/golangci-lint run ./...

##@ Build
.PHONY: build
build: envoy generate fmt vet ## Build manager binary.
	@echo "==> $@"
	@go build -o bin/manager main.go

.PHONY: envoy
envoy:
	@echo "==> $@"
	@./scripts/get-envoy.bash

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	@echo "==> $@"
	@go run ./main.go

.PHONY: docker-build
docker-build: test ## Build docker image with the manager.
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
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	@echo "==> $@"
	@$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	@echo "==> $@"
	@$(KUSTOMIZE) build config/crd | kubectl delete -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
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
	@rm -rf pomerium/envoy/bin/* $(LOCALBIN) testbin/

KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

## Tool Versions
KUSTOMIZE_VERSION ?= v4.5.4

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE) $(LOCALBIN) ## Download kustomize locally if necessary.
$(KUSTOMIZE):
	@echo "==> $@"
	@curl -s $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN)

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	@echo "==> $@"
	@GOARCH= GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	@echo "==> $@"
	@GOARCH= GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest
