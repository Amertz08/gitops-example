# Workflow History Fixtures

This directory holds recorded workflow history JSON files used by the replay tests in `replay_test.go`.

## Generating History Files

Run each workflow against a local Temporal dev server, then export its history:

```bash
# Start the dev server and worker
temporal server start-dev
go run ./cmd/worker/main.go

# After a workflow completes, export its history:
temporal workflow show --workflow-id <id> --output json > spin_up_iam_history.json
temporal workflow show --workflow-id <id> --output json > spin_down_iam_history.json
temporal workflow show --workflow-id <id> --output json > spin_up_network_history.json
temporal workflow show --workflow-id <id> --output json > spin_down_network_history.json
temporal workflow show --workflow-id <id> --output json > spin_up_eks_history.json
temporal workflow show --workflow-id <id> --output json > spin_down_eks_history.json
temporal workflow show --workflow-id <id> --output json > spin_up_history.json
temporal workflow show --workflow-id <id> --output json > spin_down_history.json
```

## Running Replay Tests

```bash
RUN_REPLAY_TESTS=1 ginkgo -v --label-filter "replay" ./internal/workflows/
```
