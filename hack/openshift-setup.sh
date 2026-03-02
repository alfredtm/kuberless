#!/usr/bin/env bash
set -euo pipefail

# ─── Configuration ───────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
HELM_VALUES="${SCRIPT_DIR}/openshift-values.yaml"
CHART_DIR="${REPO_ROOT}/deploy/helm/kuberless"
NAMESPACE="kuberless-system"

# ─── Helpers ─────────────────────────────────────────────────────────────────
info()  { echo "==> $*"; }
error() { echo "ERROR: $*" >&2; exit 1; }

# ─── Teardown ────────────────────────────────────────────────────────────────
teardown() {
    info "Tearing down kuberless..."
    helm uninstall kuberless -n "${NAMESPACE}" 2>/dev/null || true
    kubectl delete namespace "${NAMESPACE}" --wait=false 2>/dev/null || true
    kubectl delete namespace knative-serving --wait=false 2>/dev/null || true
    kubectl delete namespace kourier-system --wait=false 2>/dev/null || true
    info "Teardown complete."
}

# ─── Prerequisites ───────────────────────────────────────────────────────────
check_prerequisites() {
    local missing=()
    for cmd in helm kubectl; do
        if ! command -v "$cmd" &>/dev/null; then
            missing+=("$cmd")
        fi
    done
    if [[ ${#missing[@]} -gt 0 ]]; then
        error "Missing required tools: ${missing[*]}"
    fi

    kubectl cluster-info &>/dev/null || error "Cannot connect to cluster. Check your kubeconfig."
}

# ─── OpenShift SCC for Kourier ───────────────────────────────────────────────
# Kourier gateway hardcodes runAsUser 65534 + seccomp annotations that fall
# outside OpenShift's restricted SCC. Grant the necessary SCC to the default
# SA in knative-serving so the gateway pod can schedule.
patch_kourier_scc() {
    if ! command -v oc &>/dev/null; then
        info "oc not found, skipping SCC patch (not on OpenShift?)"
        return
    fi
    info "Granting privileged SCC to knative-serving default SA (for Kourier gateway)..."
    oc adm policy add-scc-to-user privileged -z default -n knative-serving 2>/dev/null || true

    info "Waiting for Kourier gateway to become ready..."
    for i in $(seq 1 30); do
        if kubectl get deployment 3scale-kourier-gateway -n knative-serving &>/dev/null; then
            kubectl rollout restart deploy/3scale-kourier-gateway -n knative-serving 2>/dev/null || true
            kubectl rollout status deploy/3scale-kourier-gateway -n knative-serving --timeout=120s 2>/dev/null || true
            return
        fi
        sleep 5
    done
    info "Kourier gateway not found after 150s. Run 'helm upgrade' to retry."
}

# ─── Install ─────────────────────────────────────────────────────────────────
install() {
    info "Building Helm chart dependencies..."
    helm dependency build "${CHART_DIR}"

    # Delete aggregated ClusterRoles that conflict with Helm's server-side apply.
    # These are auto-managed by the K8s aggregation controller and get recreated.
    kubectl delete clusterrole knative-serving-operator-aggregated-stable 2>/dev/null || true
    kubectl delete clusterrole knative-eventing-operator-aggregated-stable 2>/dev/null || true

    info "Installing kuberless via Helm..."
    helm upgrade --install kuberless "${CHART_DIR}" \
        --namespace "${NAMESPACE}" \
        --create-namespace \
        --values "${HELM_VALUES}" \
        --timeout 10m

    patch_kourier_scc
}

# ─── Verify ──────────────────────────────────────────────────────────────────
verify() {
    echo ""
    info "Verifying installation..."
    echo ""
    echo "--- Pods in ${NAMESPACE} ---"
    kubectl get pods -n "${NAMESPACE}"
    echo ""
    echo "--- Pods in knative-serving ---"
    kubectl get pods -n knative-serving 2>/dev/null || echo "(not ready yet)"
    echo ""
    echo "--- KnativeServing ---"
    kubectl get knativeserving -n knative-serving 2>/dev/null || echo "(not ready yet)"
    echo ""
    echo "--- HTTPRoutes ---"
    kubectl get httproute -A 2>/dev/null || echo "(none)"
    echo ""
}

print_access_info() {
    echo "============================================"
    echo " OpenShift deployment is ready!"
    echo "============================================"
    echo ""
    echo "Access via port-forward:"
    echo "  kubectl port-forward -n ${NAMESPACE} svc/kuberless-apiserver 8080:8080"
    echo "  kubectl port-forward -n ${NAMESPACE} svc/kuberless-frontend 3000:3000"
    echo ""
    echo "Smoke test:"
    echo "  kubectl apply -f ${REPO_ROOT}/deploy/samples/tenant.yaml"
    echo "  kubectl get tenants"
    echo ""
    echo "Teardown:"
    echo "  $0 --delete"
    echo ""
}

# ─── Main ────────────────────────────────────────────────────────────────────
main() {
    case "${1:-}" in
        --delete|--teardown|--destroy)
            teardown
            exit 0
            ;;
        --patch-only)
            patch_kourier_scc
            exit 0
            ;;
        --help|-h)
            echo "Usage: $0 [--delete|--patch-only]"
            echo "  (no args)     Install platform and all dependencies via Helm"
            echo "  --delete      Tear down the platform"
            echo "  --patch-only  Only apply the Kourier SCC patch (for use after helm install)"
            exit 0
            ;;
    esac

    check_prerequisites
    install
    verify
    print_access_info
}

main "$@"
