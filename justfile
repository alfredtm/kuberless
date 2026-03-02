set shell := ["bash", "-euo", "pipefail", "-c"]
set dotenv-load := true

# ─── Variables ───────────────────────────────────────────────────────────────

namespace        := env_var_or_default("NAMESPACE", "kuberless-system")
release          := env_var_or_default("RELEASE", "kuberless")
host             := env_var_or_default("HOST", "kuberless.example.com")
pg_password      := env_var_or_default("PG_PASSWORD", "kuberless")
admin_password   := env_var_or_default("ADMIN_PASSWORD", "admin")
openshift        := env_var_or_default("OPENSHIFT", "false")
container_tool   := env_var_or_default("CONTAINER_TOOL", "docker")
platforms        := env_var_or_default("PLATFORMS", "linux/arm64,linux/amd64")
kind_cluster     := env_var_or_default("KIND_CLUSTER_NAME", "kuberless-dev")
frontend_img     := env_var_or_default("FRONTEND_IMG", "kuberless-frontend:latest")

# Dependency versions
cert_manager_ver         := "v1.17.2"
knative_op_ver           := "v1.16.1"
capsule_ver              := "0.12.4"

# Tool versions
kustomize_version        := "v5.6.0"
controller_tools_version := "v0.17.2"
golangci_lint_version    := "v2.8.0"

# Local bin directory
localbin := justfile_directory() / "bin"

# ─── Default ─────────────────────────────────────────────────────────────────

[private]
default:
    @just --list

# ─── Development ─────────────────────────────────────────────────────────────

# Generate CRD manifests and copy to Helm chart
manifests: controller-gen
    {{localbin}}/controller-gen-{{controller_tools_version}} rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=operator/config/crd/bases
    cp operator/config/crd/bases/*.yaml deploy/helm/kuberless/crds/

# Generate DeepCopy methods
generate: controller-gen
    {{localbin}}/controller-gen-{{controller_tools_version}} object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Run go fmt
fmt:
    go fmt ./...

# Run go vet
vet:
    go vet ./...

# Run tests with envtest
test: manifests generate fmt vet setup-envtest
    #!/usr/bin/env bash
    set -euo pipefail
    k8s_version=$(go list -m -f '{{{{.Version}}}}' k8s.io/api | awk -F'[v.]' '{printf "1.%d", $3}')
    assets=$({{localbin}}/setup-envtest use "$k8s_version" --bin-dir {{localbin}} -p path)
    KUBEBUILDER_ASSETS="$assets" go test $(go list ./... | grep -v /e2e) -coverprofile cover.out

# Run golangci-lint
lint: golangci-lint
    {{localbin}}/golangci-lint-{{golangci_lint_version}} run ./...

# Run golangci-lint with auto-fix
lint-fix: golangci-lint
    {{localbin}}/golangci-lint-{{golangci_lint_version}} run --fix ./...

# ─── Build ───────────────────────────────────────────────────────────────────

# Build operator binary
build: manifests generate fmt vet
    go build -o bin/manager ./operator

# Build API server binary
build-apiserver: fmt vet
    go build -o bin/apiserver ./apiserver

# Build CLI binary
build-cli: fmt vet
    go build -o bin/kuberless ./cli

# Build all Go binaries
build-all: build build-apiserver build-cli

# Run the operator locally
run: manifests generate fmt vet
    go run ./operator

# Run the API server locally
run-apiserver:
    go run ./apiserver

# Run the frontend dev server
run-frontend:
    cd frontend && npm run dev

# ─── Images ──────────────────────────────────────────────────────────────────

# Build all Go images locally with ko (tagged :latest)
ko-build-local:
    KO_DOCKER_REPO=ko.local ko build ./operator --bare --platform={{platforms}} --tags=latest
    KO_DOCKER_REPO=ko.local ko build ./apiserver --bare --platform={{platforms}} --tags=latest
    KO_DOCKER_REPO=ko.local ko build ./cli --bare --platform={{platforms}} --tags=latest

# Build and push all Go images with ko (git SHA + latest tags)
ko-build:
    #!/usr/bin/env bash
    set -euo pipefail
    sha=$(git rev-parse --short HEAD)
    ko build ./operator --bare --platform={{platforms}} --tags="${sha},latest"
    ko build ./apiserver --bare --platform={{platforms}} --tags="${sha},latest"
    ko build ./cli --bare --platform={{platforms}} --tags="${sha},latest"

# Build frontend Docker image
docker-build-frontend:
    {{container_tool}} build -f frontend/Dockerfile -t {{frontend_img}} frontend

# ─── Kind ────────────────────────────────────────────────────────────────────

# Create a kind cluster with Cilium CNI
kind-cluster:
    hack/kind-setup.sh

# Delete the kind cluster
kind-teardown:
    hack/kind-setup.sh --delete

# Build and load all images into kind
kind-load: ko-build-local docker-build-frontend
    kind load docker-image ko.local/operator:latest --name {{kind_cluster}}
    kind load docker-image ko.local/apiserver:latest --name {{kind_cluster}}
    kind load docker-image ko.local/cli:latest --name {{kind_cluster}}
    kind load docker-image {{frontend_img}} --name {{kind_cluster}}

# Full kind dev environment: cluster, prereqs, images, deploy, patch
kind-dev:
    #!/usr/bin/env bash
    set -euo pipefail
    just kind-cluster
    just deploy-prereqs
    just kind-load
    just _deploy-kuberless-kind
    just _patch-kourier-nodeport

# Build, load, and restart deployments in kind
kind-redeploy: kind-load
    kubectl rollout restart deployment -n {{namespace}}

# ─── Cluster Installation ────────────────────────────────────────────────────

# Install all prerequisites and kuberless
deploy: deploy-prereqs deploy-kuberless

# Install all prerequisites
deploy-prereqs: deploy-certmanager deploy-knative deploy-capsule deploy-postgresql

# Install cert-manager
deploy-certmanager:
    helm upgrade --install cert-manager \
      oci://quay.io/jetstack/charts/cert-manager \
      --version {{cert_manager_ver}} \
      --namespace cert-manager --create-namespace \
      --set crds.enabled=true \
      --wait --timeout 5m

# Install Knative operator and create knative-serving namespace
deploy-knative:
    #!/usr/bin/env bash
    set -euo pipefail
    # The aggregation controller co-owns .rules on these ClusterRoles, causing
    # an SSA conflict on helm upgrade. Delete them first so helm can recreate
    # them cleanly; the aggregation controller repopulates .rules afterwards.
    kubectl delete clusterrole \
      knative-serving-operator-aggregated-stable \
      knative-eventing-operator-aggregated-stable \
      --ignore-not-found 2>/dev/null || true
    helm upgrade --install knative-operator \
      --repo https://knative.github.io/operator knative-operator \
      --version {{knative_op_ver}} \
      --namespace knative-operator --create-namespace \
      --wait --timeout 5m
    # Create namespace ahead of time so we can apply OpenShift SCC before Knative pods start
    kubectl create namespace knative-serving --dry-run=client -o yaml | kubectl apply -f -
    if [[ "{{openshift}}" == "true" ]]; then
      echo "Configuring OpenShift SCC for Kourier..."
      kubectl apply -f deploy/openshift/kourier-scc.yaml
    fi
    # Label the namespace so Helm can adopt it when we install the kuberless chart
    kubectl label namespace knative-serving app.kubernetes.io/managed-by=Helm --overwrite
    kubectl annotate namespace knative-serving \
      meta.helm.sh/release-name={{release}} \
      meta.helm.sh/release-namespace={{namespace}} --overwrite

# Install Capsule (namespace multi-tenancy)
deploy-capsule:
    #!/usr/bin/env bash
    set -euo pipefail
    if [[ "{{openshift}}" == "true" ]]; then
      echo "OpenShift: applying Capsule CRDs directly..."
      rm -rf /tmp/capsule-chart
      helm pull --repo https://projectcapsule.github.io/charts capsule \
        --version {{capsule_ver}} --untar --untardir /tmp/capsule-chart
      kubectl apply -f /tmp/capsule-chart/capsule/crds/
      helm upgrade --install capsule \
        --repo https://projectcapsule.github.io/charts capsule \
        --version {{capsule_ver}} \
        --namespace capsule-system --create-namespace \
        --set fullnameOverride=capsule \
        --no-hooks \
        --wait --timeout 5m
      echo "Waiting for Capsule webhook to be ready..."
      kubectl rollout status deployment/capsule-controller-manager -n capsule-system --timeout=2m
      kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=capsule -n capsule-system --timeout=2m
      sleep 10
    else
      helm upgrade --install capsule \
        --repo https://projectcapsule.github.io/charts capsule \
        --version {{capsule_ver}} \
        --namespace capsule-system --create-namespace \
        --set fullnameOverride=capsule \
        --wait --timeout 5m
    fi

# Deploy PostgreSQL and create the DB connection secret
deploy-postgresql:
    #!/usr/bin/env bash
    set -euo pipefail
    kubectl create namespace {{namespace}} --dry-run=client -o yaml | kubectl apply -f -
    sed 's/PLACEHOLDER_PG_PASSWORD/{{pg_password}}/g' deploy/postgresql.yaml | kubectl apply -n {{namespace}} -f -
    kubectl rollout status statefulset/postgresql -n {{namespace}} --timeout=5m
    kubectl create secret generic kuberless-db \
      --namespace {{namespace}} \
      --from-literal=database-url="postgres://kuberless:{{pg_password}}@postgresql.{{namespace}}.svc.cluster.local:5432/kuberless?sslmode=disable" \
      --dry-run=client -o yaml | kubectl apply -f -

# Install the kuberless platform chart
deploy-kuberless:
    #!/usr/bin/env bash
    set -euo pipefail
    printf 'ingress:\n  hosts:\n    - host: %s\n      paths:\n        - path: /api\n          service: apiserver\n        - path: /\n          service: frontend\n' "{{host}}" > /tmp/kuberless-ingress.yaml
    helm upgrade --install {{release}} deploy/helm/kuberless \
      --namespace {{namespace}} --create-namespace \
      --set global.auth.adminLogin.password={{admin_password}} \
      -f /tmp/kuberless-ingress.yaml \
      --wait --timeout 5m

# Upgrade the kuberless platform (reuse existing values)
deploy-upgrade:
    helm upgrade {{release}} deploy/helm/kuberless \
      --namespace {{namespace}} \
      --reuse-values \
      --wait --timeout 5m

# Uninstall kuberless (keeps prereqs and data)
deploy-teardown:
    helm uninstall {{release}} --namespace {{namespace}} || true

# Uninstall everything including all prereqs
deploy-teardown-all:
    #!/usr/bin/env bash
    set -euo pipefail
    helm uninstall {{release}} --namespace {{namespace}} || true
    kubectl delete -f deploy/postgresql.yaml --ignore-not-found || true
    kubectl delete secret kuberless-db -n {{namespace}} --ignore-not-found || true
    kubectl delete pvc postgresql-data -n {{namespace}} --ignore-not-found || true
    helm uninstall capsule --namespace capsule-system || true
    kubectl delete -f deploy/openshift/kourier-scc.yaml 2>/dev/null || true
    kubectl delete namespace knative-serving --ignore-not-found || true
    helm uninstall knative-operator --namespace knative-operator || true
    helm uninstall cert-manager --namespace cert-manager || true

# ─── OpenShift ───────────────────────────────────────────────────────────────

# Deploy kuberless on OpenShift (prereqs + chart + SCC patches)
openshift-deploy:
    #!/usr/bin/env bash
    set -euo pipefail
    OPENSHIFT=true just deploy-prereqs
    just _deploy-kuberless-openshift
    hack/openshift-setup.sh --patch-only || true

# Tear down the platform from OpenShift
openshift-teardown:
    hack/openshift-setup.sh --delete

# ─── Helm ────────────────────────────────────────────────────────────────────

# Install the kuberless Helm chart (simple)
helm-install:
    helm install {{release}} deploy/helm/kuberless -n {{namespace}} --create-namespace

# Upgrade the kuberless Helm chart (simple)
helm-upgrade:
    helm upgrade {{release}} deploy/helm/kuberless -n {{namespace}}

# Uninstall the kuberless Helm chart (simple)
helm-uninstall:
    helm uninstall {{release}} -n {{namespace}}

# ─── CRDs ────────────────────────────────────────────────────────────────────

# Install CRDs into the cluster
install: manifests kustomize
    {{localbin}}/kustomize-{{kustomize_version}} build operator/config/crd | kubectl apply -f -

# Uninstall CRDs from the cluster
uninstall: manifests kustomize
    {{localbin}}/kustomize-{{kustomize_version}} build operator/config/crd | kubectl delete --ignore-not-found -f -

# ─── Private Recipes ─────────────────────────────────────────────────────────

[private]
_deploy-kuberless-kind:
    helm upgrade --install {{release}} deploy/helm/kuberless \
      --namespace {{namespace}} --create-namespace \
      --values hack/kind-values.yaml \
      --set global.auth.adminLogin.password={{admin_password}} \
      --wait --timeout 10m

[private]
_deploy-kuberless-openshift:
    #!/usr/bin/env bash
    set -euo pipefail
    helm upgrade --install {{release}} deploy/helm/kuberless \
      --namespace {{namespace}} --create-namespace \
      --values hack/openshift-values.yaml \
      --set global.auth.adminLogin.password={{admin_password}} \
      --wait --timeout 10m

[private]
_patch-kourier-nodeport:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Waiting for Kourier service..."
    for i in $(seq 1 30); do
      if kubectl get svc kourier -n kourier-system &>/dev/null; then
        echo "Patching Kourier service with kind NodePort values..."
        kubectl patch service kourier -n kourier-system \
          --type merge \
          --patch '{
            "spec": {
              "ports": [
                {"name": "http2", "nodePort": 31080, "port": 80, "protocol": "TCP", "targetPort": 8080},
                {"name": "https", "nodePort": 31443, "port": 443, "protocol": "TCP", "targetPort": 8443}
              ]
            }
          }'
        exit 0
      fi
      sleep 5
    done
    echo "Kourier service not found after 150s, skipping NodePort patch."

# ─── Dependencies (tool installers) ─────────────────────────────────────────

# Install kustomize
kustomize:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p {{localbin}}
    test -f {{localbin}}/kustomize-{{kustomize_version}} && exit 0
    echo "Installing kustomize {{kustomize_version}}..."
    GOBIN={{localbin}} go install sigs.k8s.io/kustomize/kustomize/v5@{{kustomize_version}}
    mv {{localbin}}/kustomize {{localbin}}/kustomize-{{kustomize_version}}

# Install controller-gen
controller-gen:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p {{localbin}}
    test -f {{localbin}}/controller-gen-{{controller_tools_version}} && exit 0
    echo "Installing controller-gen {{controller_tools_version}}..."
    GOBIN={{localbin}} go install sigs.k8s.io/controller-tools/cmd/controller-gen@{{controller_tools_version}}
    mv {{localbin}}/controller-gen {{localbin}}/controller-gen-{{controller_tools_version}}

# Install setup-envtest
setup-envtest:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p {{localbin}}
    version=$(go list -m -f '{{{{.Version}}}}' sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $2, $3}')
    target={{localbin}}/setup-envtest
    test -f "$target" && exit 0
    echo "Installing setup-envtest ${version}..."
    GOBIN={{localbin}} go install sigs.k8s.io/controller-runtime/tools/setup-envtest@"${version}"

# Alias for setup-envtest
envtest: setup-envtest

# Install golangci-lint
golangci-lint:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p {{localbin}}
    test -f {{localbin}}/golangci-lint-{{golangci_lint_version}} && exit 0
    echo "Installing golangci-lint {{golangci_lint_version}}..."
    GOBIN={{localbin}} go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@{{golangci_lint_version}}
    mv {{localbin}}/golangci-lint {{localbin}}/golangci-lint-{{golangci_lint_version}}
