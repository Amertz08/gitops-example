package main

import (
	"log"
	"os"

	"github.com/Amertz08/gitops-example/internal/activities"
	"github.com/Amertz08/gitops-example/internal/workflows"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

const taskQueue = "infrastructure"

func main() {
	temporalHost := os.Getenv("TEMPORAL_HOST")
	if temporalHost == "" {
		temporalHost = "localhost:7233"
	}

	c, err := client.Dial(client.Options{HostPort: temporalHost})
	if err != nil {
		log.Fatalf("failed to connect to Temporal: %v", err)
	}
	defer c.Close()

	w := worker.New(c, taskQueue, worker.Options{})

	w.RegisterWorkflow(workflows.SpinUpWorkflow)
	w.RegisterWorkflow(workflows.SpinDownWorkflow)
	w.RegisterActivity(&activities.AWSActivities{})

	if err := w.Run(worker.InterruptCh()); err != nil {
		log.Fatalf("worker error: %v", err)
	}
}
