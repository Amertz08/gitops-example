# gitops-example

A GitOps-driven application demonstrating automated Kubernetes deployments via ArgoCD, with a Temporal worker for provisioning AWS infrastructure.

## Components

### API (`cmd/api/main.go`)
A small HTTP API built with [Echo v5](https://github.com/labstack/echo), listening on `:1323`.

### Temporal Worker (`cmd/worker/main.go`)
A [Temporal](https://temporal.io) worker that executes infrastructure provisioning workflows against AWS. It registers on the `infrastructure` task queue.

## Temporal Workflows

Workflows are composed as parent/child chains, each owning a distinct layer of infrastructure:

| Workflow | Direction | Responsibility |
|---|---|---|
| `SpinUpWorkflow` | up | Orchestrates full environment creation |
| `SpinUpIAMWorkflow` | up | Creates EKS cluster and node IAM roles |
| `SpinUpNetworkWorkflow` | up | Creates VPC, subnets, internet gateway, route tables |
| `SpinUpEKSWorkflow` | up | Creates EKS cluster and node group |
| `SpinDownWorkflow` | down | Orchestrates full environment teardown |
| `SpinDownEKSWorkflow` | down | Deletes node group and EKS cluster |
| `SpinDownNetworkWorkflow` | down | Deletes subnets, internet gateway, and VPC |
| `SpinDownIAMWorkflow` | down | Deletes EKS cluster and node IAM roles |

## Configuration

| Environment Variable | Default | Description |
|---|---|---|
| `TEMPORAL_HOST` | `localhost:7233` | Temporal server address |
| `AWS_ROLE_ARN` | _(unset)_ | IAM role to assume for AWS API calls. Falls back to the default credential chain when unset. |

## Development

```bash
# Run the API locally
go run ./cmd/api/main.go

# Run the worker locally
go run ./cmd/worker/main.go

# Build the API binary
go build -o api ./cmd/api/

# Build the worker binary
go build -o worker ./cmd/worker/

# Run tests
go test -v ./...

# Build Docker images
docker build -t gitops-example .
docker build --target worker-release-stage -t gitops-example-worker .
```

## CI/CD

GitHub Actions builds and pushes Docker images to `amertz08/gitops-example` on Docker Hub. Workflows are split into reusable components under `.github/workflows/`:

- **`docker-build-push-api.yml`** — triggers on changes to `cmd/api/`
- **`docker-build-push-worker.yml`** — triggers on changes to `cmd/worker/` or `internal/`
- On push to `main`: updates the prod image tag in `deploy/overlays/prod/kustomization.yaml`
- On push to a feature branch: registers an ArgoCD Application under `deploy/argocd/branches/`
- On branch deletion: removes the corresponding ArgoCD Application

## Deployment

Kubernetes manifests live under `deploy/` and are managed with [Kustomize](https://kustomize.io):

```
deploy/
├── base/            # shared Deployment, Service, and Worker manifests
├── overlays/prod/   # production image tags and replica count
└── argocd/          # ArgoCD Application resources (prod + per-branch)
```

[ArgoCD](https://argo-cd.readthedocs.io) syncs the cluster state from this repository. Branch deployments are created automatically by CI and cleaned up on branch deletion.
