package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type SpinUpInput struct {
	Region           string
	ClusterName      string
	NodeCount        int32
	NodeInstanceType string
	Environment      string
	Team             string
}

type SpinDownInput struct {
	Region      string
	ClusterName string
	VpcID       string
}

var activityOptions = workflow.ActivityOptions{
	StartToCloseTimeout: 30 * time.Minute,
	RetryPolicy: &temporal.RetryPolicy{
		MaximumAttempts: 3,
		InitialInterval: 10 * time.Second,
	},
}

func (i SpinUpInput) validate() error {
	switch {
	case i.Region == "":
		return fmt.Errorf("Region is required")
	case i.ClusterName == "":
		return fmt.Errorf("ClusterName is required")
	case i.NodeInstanceType == "":
		return fmt.Errorf("NodeInstanceType is required")
	case i.Environment == "":
		return fmt.Errorf("Environment is required")
	case i.Team == "":
		return fmt.Errorf("Team is required")
	case i.NodeCount <= 0:
		return fmt.Errorf("NodeCount must be greater than 0")
	}
	return nil
}

func SpinUpWorkflow(ctx workflow.Context, input SpinUpInput) error {
	if err := input.validate(); err != nil {
		return temporal.NewNonRetryableApplicationError(err.Error(), "InvalidInput", err)
	}

	var network SpinUpNetworkOutput
	if err := workflow.ExecuteChildWorkflow(ctx, SpinUpNetworkWorkflow, SpinUpNetworkInput{
		Region:      input.Region,
		Environment: input.Environment,
		Team:        input.Team,
	}).Get(ctx, &network); err != nil {
		return err
	}

	return workflow.ExecuteChildWorkflow(ctx, SpinUpEKSWorkflow, SpinUpEKSInput{
		Region:           input.Region,
		ClusterName:      input.ClusterName,
		VpcID:            network.VpcID,
		SubnetIDs:        network.SubnetIDs,
		NodeCount:        input.NodeCount,
		NodeInstanceType: input.NodeInstanceType,
		Environment:      input.Environment,
		Team:             input.Team,
	}).Get(ctx, nil)
}

func (i SpinDownInput) validate() error {
	switch {
	case i.Region == "":
		return fmt.Errorf("Region is required")
	case i.ClusterName == "":
		return fmt.Errorf("ClusterName is required")
	case i.VpcID == "":
		return fmt.Errorf("VpcID is required")
	}
	return nil
}

func SpinDownWorkflow(ctx workflow.Context, input SpinDownInput) error {
	if err := input.validate(); err != nil {
		return temporal.NewNonRetryableApplicationError(err.Error(), "InvalidInput", err)
	}

	if err := workflow.ExecuteChildWorkflow(ctx, SpinDownEKSWorkflow, SpinDownEKSInput{
		Region:      input.Region,
		ClusterName: input.ClusterName,
	}).Get(ctx, nil); err != nil {
		return err
	}

	return workflow.ExecuteChildWorkflow(ctx, SpinDownNetworkWorkflow, SpinDownNetworkInput{
		Region: input.Region,
		VpcID:  input.VpcID,
	}).Get(ctx, nil)
}
