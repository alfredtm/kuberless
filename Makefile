FRONTEND_IMG ?= kuberless-frontend:latest

ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

CONTAINER_TOOL ?= docker
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate CRD manifests.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=operator/config/crd/bases
	cp operator/config/crd/bases/*.yaml deploy/helm/kuberless/crds/

.PHONY: generate
generate: controller-gen ## Generate DeepCopy methods.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet setup-envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: lint
lint: golangci-lint ## Run golangci-lint.
	$(GOLANGCI_LINT) run ./...

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint with auto-fix.
	$(GOLANGCI_LINT) run --fix ./...

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build operator binary.
	go build -o bin/manager ./operator

.PHONY: build-apiserver
build-apiserver: fmt vet ## Build API server binary.
	go build -o bin/apiserver ./apiserver

.PHONY: build-cli
build-cli: fmt vet ## Build CLI binary.
	go build -o bin/kuberless ./cli

.PHONY: build-all
build-all: build build-apiserver build-cli ## Build all Go binaries.

.PHONY: run
run: manifests generate fmt vet ## Run the operator locally.
	go run ./operator

.PHONY: run-apiserver
run-apiserver: ## Run the API server locally.
	go run ./apiserver

.PHONY: run-frontend
run-frontend: ## Run the frontend dev server.
	cd frontend && npm run dev

##@ Images

KO ?= ko
PLATFORMS ?= linux/arm64,linux/amd64

.PHONY: ko-build-local
ko-build-local: ## Build all Go images locally with ko.
	KO_DOCKER_REPO=ko.local $(KO) build ./operator --bare --platform=$(PLATFORMS)
	KO_DOCKER_REPO=ko.local $(KO) build ./apiserver --bare --platform=$(PLATFORMS)
	KO_DOCKER_REPO=ko.local $(KO) build ./cli --bare --platform=$(PLATFORMS)

.PHONY: ko-build
ko-build: ## Build and push all Go images with ko.
	$(KO) build ./operator --bare --platform=$(PLATFORMS) --tags=$$(git rev-parse --short HEAD),latest
	$(KO) build ./apiserver --bare --platform=$(PLATFORMS) --tags=$$(git rev-parse --short HEAD),latest
	$(KO) build ./cli --bare --platform=$(PLATFORMS) --tags=$$(git rev-parse --short HEAD),latest

.PHONY: docker-build-frontend
docker-build-frontend: ## Build frontend docker image.
	$(CONTAINER_TOOL) build -f frontend/Dockerfile -t ${FRONTEND_IMG} frontend

##@ Helm

.PHONY: helm-install
helm-install: ## Install the kuberless Helm chart.
	helm install kuberless deploy/helm/kuberless -n kuberless-system --create-namespace

.PHONY: helm-upgrade
helm-upgrade: ## Upgrade the kuberless Helm chart.
	helm upgrade kuberless deploy/helm/kuberless -n kuberless-system

.PHONY: helm-uninstall
helm-uninstall: ## Uninstall the kuberless Helm chart.
	helm uninstall kuberless -n kuberless-system

##@ Cluster Installation

# Configurable variables — override on the command line or in a .env file.
NAMESPACE        ?= kuberless-system
RELEASE          ?= kuberless
HOST             ?= kuberless.example.com
JWT_SECRET       ?= change-me-in-production
PG_PASSWORD      ?= kuberless
ADMIN_PASSWORD   ?= admin
OPENSHIFT        ?= false

# Dependency versions
CERT_MANAGER_VER ?= v1.17.2
KNATIVE_OP_VER   ?= v1.16.1
CAPSULE_VER      ?= 0.12.4
PG_VER           ?= 16.4.3

.PHONY: deploy
deploy: deploy-prereqs deploy-kuberless ## Install all prerequisites and kuberless.

.PHONY: deploy-prereqs
deploy-prereqs: deploy-certmanager deploy-knative deploy-capsule deploy-postgresql ## Install all prerequisites.

.PHONY: deploy-certmanager
deploy-certmanager: ## Install cert-manager.
	helm upgrade --install cert-manager \
	  oci://quay.io/jetstack/charts/cert-manager \
	  --version $(CERT_MANAGER_VER) \
	  --namespace cert-manager --create-namespace \
	  --set crds.enabled=true \
	  --wait --timeout 5m

.PHONY: deploy-knative
deploy-knative: ## Install Knative operator and create knative-serving namespace.
	helm upgrade --install knative-operator \
	  --repo https://knative.github.io/operator knative-operator \
	  --version $(KNATIVE_OP_VER) \
	  --namespace knative-operator --create-namespace \
	  --wait --timeout 5m
	@# Create namespace ahead of time so we can apply OpenShift SCC before Knative pods start.
	kubectl create namespace knative-serving --dry-run=client -o yaml | kubectl apply -f -
	@if [ "$(OPENSHIFT)" = "true" ]; then \
	  echo "Configuring OpenShift SCC for Kourier..."; \
	  kubectl apply -f deploy/openshift/kourier-scc.yaml; \
	fi
	@# Label the namespace so Helm can adopt it when we install the kuberless chart.
	kubectl label namespace knative-serving app.kubernetes.io/managed-by=Helm --overwrite
	kubectl annotate namespace knative-serving \
	  meta.helm.sh/release-name=$(RELEASE) \
	  meta.helm.sh/release-namespace=$(NAMESPACE) --overwrite

.PHONY: deploy-capsule
deploy-capsule: ## Install Capsule (namespace multi-tenancy).
	@# On OpenShift, Capsule's hook Jobs use deprecated seccomp annotations that
	@# OpenShift 4.14+ rejects. Apply CRDs directly and skip hooks instead.
	@if [ "$(OPENSHIFT)" = "true" ]; then \
	  echo "OpenShift: applying Capsule CRDs directly..."; \
	  helm pull --repo https://projectcapsule.github.io/charts capsule \
	    --version $(CAPSULE_VER) --untar --untardir /tmp/capsule-chart; \
	  kubectl apply -f /tmp/capsule-chart/capsule/crds/; \
	  helm upgrade --install capsule \
	    --repo https://projectcapsule.github.io/charts capsule \
	    --version $(CAPSULE_VER) \
	    --namespace capsule-system --create-namespace \
	    --set fullnameOverride=capsule \
	    --no-hooks \
	    --wait --timeout 5m; \
	else \
	  helm upgrade --install capsule \
	    --repo https://projectcapsule.github.io/charts capsule \
	    --version $(CAPSULE_VER) \
	    --namespace capsule-system --create-namespace \
	    --set fullnameOverride=capsule \
	    --wait --timeout 5m; \
	fi

.PHONY: deploy-postgresql
deploy-postgresql: ## Deploy PostgreSQL (official image) and create the DB connection secret.
	@kubectl create namespace $(NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	@sed 's/PLACEHOLDER_PG_PASSWORD/$(PG_PASSWORD)/g' deploy/postgresql.yaml | kubectl apply -n $(NAMESPACE) -f -
	@kubectl rollout status statefulset/postgresql -n $(NAMESPACE) --timeout=5m
	kubectl create secret generic kuberless-db \
	  --namespace $(NAMESPACE) \
	  --from-literal=database-url="postgres://kuberless:$(PG_PASSWORD)@postgresql.$(NAMESPACE).svc.cluster.local:5432/kuberless?sslmode=disable" \
	  --dry-run=client -o yaml | kubectl apply -f -

.PHONY: deploy-kuberless
deploy-kuberless: ## Install the kuberless platform chart.
	@printf 'ingress:\n  hosts:\n    - host: %s\n      paths:\n        - path: /api\n          service: apiserver\n        - path: /\n          service: frontend\n' "$(HOST)" > /tmp/kuberless-ingress.yaml
	helm upgrade --install $(RELEASE) deploy/helm/kuberless \
	  --namespace $(NAMESPACE) --create-namespace \
	  --set global.auth.adminLogin.password=$(ADMIN_PASSWORD) \
	  --set apiserver.jwtSecret=$(JWT_SECRET) \
	  -f /tmp/kuberless-ingress.yaml \
	  --wait --timeout 5m

.PHONY: deploy-upgrade
deploy-upgrade: ## Upgrade the kuberless platform (reuse existing values).
	helm upgrade $(RELEASE) deploy/helm/kuberless \
	  --namespace $(NAMESPACE) \
	  --reuse-values \
	  --wait --timeout 5m

.PHONY: deploy-teardown
deploy-teardown: ## Uninstall kuberless (keeps prereqs and data).
	helm uninstall $(RELEASE) --namespace $(NAMESPACE) || true

.PHONY: deploy-teardown-all
deploy-teardown-all: deploy-teardown ## Uninstall everything including all prereqs.
	kubectl delete -f deploy/postgresql.yaml --ignore-not-found || true
	kubectl delete secret kuberless-db -n $(NAMESPACE) --ignore-not-found || true
	kubectl delete pvc postgresql-data -n $(NAMESPACE) --ignore-not-found || true
	helm uninstall capsule --namespace capsule-system || true
	kubectl delete -f deploy/openshift/kourier-scc.yaml 2>/dev/null || true
	kubectl delete namespace knative-serving --ignore-not-found || true
	helm uninstall knative-operator --namespace knative-operator || true
	helm uninstall cert-manager --namespace cert-manager || true

##@ CRDs

.PHONY: install
install: manifests kustomize ## Install CRDs into cluster.
	$(KUSTOMIZE) build operator/config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from cluster.
	$(KUSTOMIZE) build operator/config/crd | $(KUBECTL) delete --ignore-not-found -f -

##@ OpenShift

.PHONY: openshift-setup
openshift-setup: ## Set up the platform on OpenShift.
	hack/openshift-setup.sh

.PHONY: openshift-teardown
openshift-teardown: ## Tear down the platform from OpenShift.
	hack/openshift-setup.sh --delete

##@ Kind

KIND_CLUSTER_NAME ?= kuberless-dev

.PHONY: kind-setup
kind-setup: ## Create a kind cluster with dependencies.
	hack/kind-setup.sh

.PHONY: kind-teardown
kind-teardown: ## Delete the kind cluster.
	hack/kind-setup.sh --delete

.PHONY: kind-load
kind-load: ko-build-local docker-build-frontend ## Build and load images into kind.
	$(KIND) load docker-image ko.local/operator:latest --name $(KIND_CLUSTER_NAME)
	$(KIND) load docker-image ko.local/apiserver:latest --name $(KIND_CLUSTER_NAME)
	$(KIND) load docker-image ko.local/cli:latest --name $(KIND_CLUSTER_NAME)
	$(KIND) load docker-image kuberless-frontend:latest --name $(KIND_CLUSTER_NAME)

.PHONY: kind-redeploy
kind-redeploy: kind-load ## Build, load, and restart deployments in kind.
	$(KUBECTL) rollout restart deployment -n kuberless-system

##@ Dependencies

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

KUBECTL ?= kubectl
KIND ?= kind
KUSTOMIZE ?= $(LOCALBIN)/kustomize-$(KUSTOMIZE_VERSION)
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen-$(CONTROLLER_TOOLS_VERSION)
ENVTEST ?= $(LOCALBIN)/setup-envtest-$(ENVTEST_VERSION)
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint-$(GOLANGCI_LINT_VERSION)

KUSTOMIZE_VERSION ?= v5.6.0
CONTROLLER_TOOLS_VERSION ?= v0.17.2
ENVTEST_VERSION ?= $(shell go list -m -f "{{ .Version }}" sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')
ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')
GOLANGCI_LINT_VERSION ?= v2.8.0

.PHONY: kustomize
kustomize: $(KUSTOMIZE)
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN)
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path

.PHONY: envtest
envtest: $(ENVTEST)
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT)
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,${GOLANGCI_LINT_VERSION})

define go-install-tool
@[ -f $(1) ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv "$$(echo "$(1)" | sed "s/-$(3)$$//")" $(1) ;\
}
endef
