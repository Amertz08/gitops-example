package workflows

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type SpinUpInput struct {
	Region           string
	ClusterName      string
	NodeCount        int32
	NodeInstanceType string
}

type SpinDownInput struct {
	Region      string
	ClusterName string
}

var activityOptions = workflow.ActivityOptions{
	StartToCloseTimeout: 30 * time.Minute,
	RetryPolicy: &temporal.RetryPolicy{
		MaximumAttempts: 3,
		InitialInterval: 10 * time.Second,
	},
}

func SpinUpWorkflow(ctx workflow.Context, input SpinUpInput) error {
	var network SpinUpNetworkOutput
	if err := workflow.ExecuteChildWorkflow(ctx, SpinUpNetworkWorkflow, SpinUpNetworkInput{
		Region: input.Region,
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
	}).Get(ctx, nil)
}

func SpinDownWorkflow(ctx workflow.Context, input SpinDownInput) error {
	var vpcID string
	if err := workflow.ExecuteChildWorkflow(ctx, SpinDownEKSWorkflow, SpinDownEKSInput{
		Region:      input.Region,
		ClusterName: input.ClusterName,
	}).Get(ctx, &vpcID); err != nil {
		return err
	}

	return workflow.ExecuteChildWorkflow(ctx, SpinDownNetworkWorkflow, SpinDownNetworkInput{
		Region: input.Region,
		VpcID:  vpcID,
	}).Get(ctx, nil)
}
