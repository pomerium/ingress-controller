# kubeenv is not supported on darwin/arm64
ifeq (darwin arm64,$(shell go env GOOS GOARCH))
KUBEENV_GOARCH=amd64
else
KUBEENV_GOARCH=$(shell go env GOARCH)
endif

CRD_PACKAGE=github.com/pomerium/ingress-controller/apis/ingress/v1

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

# Pomerium core requires a special tag set to indicate
# the embedded resources would be supplied externally
GOTAGS = -tags embed_pomerium

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
	@$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role crd webhook paths=$(CRD_PACKAGE) output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	@echo "==> $@"
	@$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths=$(CRD_PACKAGE)
	@go generate $(GOTAGS) ./...
	@gofmt -s -w ./

.PHONY: fmt
fmt: ## Run go fmt against code.
	@echo "==> $@"
	@go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	@echo "==> $@"
	@go vet $(GOTAGS) ./...

.PHONY: test
test: envoy manifests generate fmt vet envtest ## Run tests.
	@echo "==> $@"
	@KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path --arch=$(KUBEENV_GOARCH))" go test $(GOTAGS) ./... -coverprofile cover.out

.PHONY: lint
lint: envoy pomerium-ui ## Verifies `golint` passes.
	@echo "==> $@"
	@go run github.com/golangci/golangci-lint/cmd/golangci-lint run ./...

##@ Build
.PHONY: build
build: pomerium-ui build-go ## Build manager binary.
	@echo "==> $@"

##@ Build
.PHONY: build-go
build-go: envoy
	@echo "==> $@"
	@go build $(GOTAGS) -o bin/manager main.go

.PHONY: envoy
envoy:
	@echo "==> $@"
	@./scripts/get-envoy.bash

UI_DIR = $(shell go list -f {{.Dir}} github.com/pomerium/pomerium/ui)
internal/ui:
	@echo "@==> $@"
	@cp -rf $(UI_DIR) ./internal
	@chmod u+w internal/ui internal/ui/dist

internal/ui/node_modules: internal/ui
	@echo "@==> $@"
	@cd internal/ui && yarn install --network-timeout 120000

.PHONY: pomerium-ui
pomerium-ui: internal/ui/dist/index.js
internal/ui/dist/index.js: internal/ui/node_modules
	@echo "==> $@"
	@cd internal/ui && yarn build

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	@echo "==> $@"
	@go run $(GOTAGS) ./main.go

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
	@rm -rf pomerium/envoy/bin/*
	@rm -rf $(LOCALBIN)
	@rm -rf testbin
	@chmod -Rf u+w internal/ui || true
	@rm -rf internal/ui

KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

## Tool Versions
KUSTOMIZE_VERSION ?= v4.5.4

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE)  ## Download kustomize locally if necessary.
	@echo "==> $@"

$(KUSTOMIZE): $(LOCALBIN)
	@echo "==> $@"
	@rm -rf $(KUSTOMIZE)
	@curl -s $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN)

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	@echo "==> $@"
	@GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.9.0

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	@echo "==> $@"
	@GOARCH= GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest

.PHONY: deployment
deployment:
	@echo "==> $@"
	@$(KUSTOMIZE) build config/default > deployment.yaml

.PHONY: docs
docs: manifests
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
	@$(KUSTOMIZE) build config/dev/local | kubectl apply --filename -

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
