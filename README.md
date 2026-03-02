# Kuberless

Self-hosted multi-tenant serverless platform on Kubernetes. Push a container image, get a URL.

Built as a learning project — no backup strategy, no monitoring, error handling that's optimistic at best. Complete enough to deploy and poke at. Read the [blog post](https://alfredtm.github.io/2026/03/02/kuberless/) for context.

## Components

- **`operator/`** — watches Tenant and App CRDs; creates namespaces, Knative Services, CiliumNetworkPolicies, ResourceQuotas, Capsule Tenants, DomainMappings
- **`apiserver/`** — REST API (Go + chi); auth, tenant/app/env/domain CRUD, syncs to CRDs and PostgreSQL
- **`cli/`** — CLI client (Go + Cobra)
- **`frontend/`** — Next.js dashboard

## Prerequisites

- Kubernetes cluster with [Cilium](https://docs.cilium.io/en/stable/gettingstarted/) >= 1.15 as CNI
- [`just`](https://just.systems) task runner
- cert-manager, Knative Serving, Capsule, and PostgreSQL — installed separately via `just deploy-prereqs`

## Install

```bash
just deploy-prereqs
just deploy-kuberless HOST=kuberless.example.com ADMIN_PASSWORD=yourpassword
```

Or with helm directly:

```bash
helm install kuberless deploy/helm/kuberless \
  -n kuberless-system --create-namespace \
  -f my-values.yaml
```

Minimal `my-values.yaml`:

```yaml
global:
  auth:
    adminLogin:
      enabled: true
      username: "admin"
      password: "yourpassword"

ingress:
  enabled: true
  className: nginx
  hosts:
    - host: kuberless.example.com
      paths:
        - path: /api
          service: apiserver
        - path: /
          service: frontend
```

The JWT signing secret is auto-generated on first install and preserved across upgrades.

```bash
just deploy-upgrade
just deploy-teardown        # removes kuberless, keeps prereqs
just deploy-teardown-all    # removes everything
```

### OpenShift

```bash
just openshift-deploy
just openshift-teardown
```

## Local development (kind)

```bash
just kind-dev       # one-shot: cluster + prereqs + images + deploy
just kind-redeploy  # rebuild and restart after code changes
just kind-teardown

kubectl port-forward -n kuberless-system svc/kuberless-apiserver 8080:8080
kubectl port-forward -n kuberless-system svc/kuberless-frontend  3000:3000
```

```bash
just build-all  # Go binaries → bin/
just test       # go test with envtest
just lint
```

## CLI

```bash
kuberless login
kuberless tenant create my-org --plan starter
kuberless deploy ghcr.io/myorg/myapp:latest --name myapp --port 8080
kuberless apps list
kuberless logs myapp --follow
kuberless env set myapp KEY=value
kuberless domains add myapp api.example.com
kuberless apps pause myapp
```

## Tenant isolation

Each tenant gets a namespace (`tenant-{name}`) with a Capsule Tenant, CiliumNetworkPolicy (cross-tenant traffic denied), and ResourceQuota:

Plans: Free / Starter / Pro / Enterprise (see `api/v1alpha1/`).

## License

Apache 2.0
