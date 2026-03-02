#!/usr/bin/env bash
set -euo pipefail

# ─── Configuration ───────────────────────────────────────────────────────────
CLUSTER_NAME="${KIND_CLUSTER_NAME:-kuberless-dev}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KIND_CONFIG="${SCRIPT_DIR}/kind-config.yaml"
CILIUM_VERSION="1.16.5"
NAMESPACE="kuberless-system"
RELEASE="kuberless"

# ─── Helpers ─────────────────────────────────────────────────────────────────
info()  { echo "==> $*"; }
error() { echo "ERROR: $*" >&2; exit 1; }

# ─── Teardown ────────────────────────────────────────────────────────────────
teardown() {
    info "Deleting kind cluster '${CLUSTER_NAME}'..."
    kind delete cluster --name "${CLUSTER_NAME}"
    info "Cluster deleted."
}

# ─── Prerequisites ───────────────────────────────────────────────────────────
check_prerequisites() {
    local missing=()
    for cmd in kind docker helm kubectl; do
        if ! command -v "$cmd" &>/dev/null; then
            missing+=("$cmd")
        fi
    done
    if [[ ${#missing[@]} -gt 0 ]]; then
        error "Missing required tools: ${missing[*]}"
    fi
}

# ─── Kind Cluster ────────────────────────────────────────────────────────────
setup_kind_cluster() {
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        info "Kind cluster '${CLUSTER_NAME}' already exists, reusing."
        kubectl cluster-info --context "kind-${CLUSTER_NAME}" &>/dev/null \
            || error "Cluster exists but is not reachable. Delete it with: $0 --delete"
    else
        info "Creating kind cluster '${CLUSTER_NAME}'..."
        kind create cluster --name "${CLUSTER_NAME}" --config "${KIND_CONFIG}"
    fi
    kubectl config use-context "kind-${CLUSTER_NAME}"
}

# ─── Cilium (CNI — must be installed before anything else) ───────────────────
install_cilium() {
    if kubectl get daemonset -n kube-system cilium &>/dev/null; then
        info "Cilium already installed, skipping."
        return
    fi

    local api_server_ip
    api_server_ip=$(docker inspect "${CLUSTER_NAME}-control-plane" \
        --format '{{ .NetworkSettings.Networks.kind.IPAddress }}')
    info "Control plane IP: ${api_server_ip}"

    info "Installing Cilium ${CILIUM_VERSION} via Helm..."
    helm repo add cilium https://helm.cilium.io/ 2>/dev/null || true
    helm repo update cilium

    helm install cilium cilium/cilium \
        --namespace kube-system \
        --version "${CILIUM_VERSION}" \
        --set kubeProxyReplacement=true \
        --set k8sServiceHost="${api_server_ip}" \
        --set k8sServicePort=6443 \
        --set operator.replicas=1 \
        --wait --timeout 5m

    info "Waiting for Cilium pods to be ready..."
    kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=cilium-agent \
        -n kube-system --timeout=300s
}

# ─── Namespaces ──────────────────────────────────────────────────────────────
# Pre-create knative-serving so Helm can adopt it via resource-policy keep
setup_namespaces() {
    kubectl create namespace knative-serving --dry-run=client -o yaml | kubectl apply -f -
    kubectl label namespace knative-serving app.kubernetes.io/managed-by=Helm --overwrite
    kubectl annotate namespace knative-serving \
        meta.helm.sh/release-name="${RELEASE}" \
        meta.helm.sh/release-namespace="${NAMESPACE}" --overwrite
}

# ─── Next Steps ──────────────────────────────────────────────────────────────
print_next_steps() {
    echo ""
    echo "============================================"
    echo " Kind cluster '${CLUSTER_NAME}' is ready!"
    echo "============================================"
    echo ""
    echo "Next steps:"
    echo "  just deploy-prereqs   # install cert-manager, knative, capsule, postgresql"
    echo "  just kind-load        # build and load images into kind"
    echo "  just _deploy-kuberless-kind  # install the platform"
    echo ""
    echo "Or run everything at once:"
    echo "  just kind-dev"
    echo ""
    echo "Teardown:"
    echo "  just kind-teardown"
    echo ""
}

# ─── Main ────────────────────────────────────────────────────────────────────
main() {
    case "${1:-}" in
        --delete|--teardown|--destroy)
            teardown
            exit 0
            ;;
        --help|-h)
            echo "Usage: $0 [--delete]"
            echo "  (no args)  Create kind cluster with Cilium CNI"
            echo "  --delete   Delete the kind cluster"
            exit 0
            ;;
    esac

    check_prerequisites
    setup_kind_cluster
    install_cilium
    setup_namespaces
    print_next_steps
}

main "$@"
