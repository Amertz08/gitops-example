package main

import (
	"log/slog"
	"os"

	"github.com/Amertz08/gitops-example/internal/activities"
	"github.com/Amertz08/gitops-example/internal/workflows"
	"go.temporal.io/sdk/client"
	sdklog "go.temporal.io/sdk/log"
	"go.temporal.io/sdk/worker"
)

const taskQueue = "infrastructure"

func main() {
	temporalHost := os.Getenv("TEMPORAL_HOST")
	if temporalHost == "" {
		temporalHost = "localhost:7233"
	}

	slogLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	temporalLogger := sdklog.NewStructuredLogger(slogLogger)

	c, err := client.Dial(client.Options{
		HostPort: temporalHost,
		Logger:   temporalLogger,
	})
	if err != nil {
		slogLogger.Error("failed to connect to Temporal", "error", err)
		os.Exit(1)
	}
	defer c.Close()

	w := worker.New(c, taskQueue, worker.Options{})

	w.RegisterWorkflow(workflows.SpinUpWorkflow)
	w.RegisterWorkflow(workflows.SpinUpNetworkWorkflow)
	w.RegisterWorkflow(workflows.SpinUpEKSWorkflow)
	w.RegisterWorkflow(workflows.SpinDownWorkflow)
	w.RegisterWorkflow(workflows.SpinDownEKSWorkflow)
	w.RegisterWorkflow(workflows.SpinDownNetworkWorkflow)
	w.RegisterActivity(activities.NewAWSActivities(os.Getenv("AWS_ROLE_ARN")))

	slogLogger.Info("worker starting", "taskQueue", taskQueue, "temporalHost", temporalHost)

	if err := w.Run(worker.InterruptCh()); err != nil {
		slogLogger.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}
