package workflows

import (
	"time"

	"github.com/Amertz08/gitops-example/internal/activities"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type SpinUpInput struct {
	Region             string
	ClusterName        string
	NodeCount          int32
	NodeInstanceType   string
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
	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	aws := &activities.AWSActivities{}

	var vpcID string
	if err := workflow.ExecuteActivity(ctx, aws.CreateVPC, input.Region).Get(ctx, &vpcID); err != nil {
		return err
	}

	var subnetIDs []string
	if err := workflow.ExecuteActivity(ctx, aws.CreateSubnets, input.Region, vpcID).Get(ctx, &subnetIDs); err != nil {
		return err
	}

	if err := workflow.ExecuteActivity(ctx, aws.CreateInternetGateway, input.Region, vpcID).Get(ctx, nil); err != nil {
		return err
	}

	if err := workflow.ExecuteActivity(ctx, aws.ConfigureRouteTables, input.Region, vpcID, subnetIDs).Get(ctx, nil); err != nil {
		return err
	}

	if err := workflow.ExecuteActivity(ctx, aws.CreateEKSCluster, input.Region, input.ClusterName, vpcID, subnetIDs).Get(ctx, nil); err != nil {
		return err
	}

	if err := workflow.ExecuteActivity(ctx, aws.CreateNodeGroup, input.Region, input.ClusterName, subnetIDs, input.NodeCount, input.NodeInstanceType).Get(ctx, nil); err != nil {
		return err
	}

	return nil
}

func SpinDownWorkflow(ctx workflow.Context, input SpinDownInput) error {
	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	aws := &activities.AWSActivities{}

	if err := workflow.ExecuteActivity(ctx, aws.DeleteNodeGroup, input.Region, input.ClusterName).Get(ctx, nil); err != nil {
		return err
	}

	var vpcID string
	if err := workflow.ExecuteActivity(ctx, aws.DeleteEKSCluster, input.Region, input.ClusterName).Get(ctx, &vpcID); err != nil {
		return err
	}

	if err := workflow.ExecuteActivity(ctx, aws.DeleteSubnets, input.Region, vpcID).Get(ctx, nil); err != nil {
		return err
	}

	if err := workflow.ExecuteActivity(ctx, aws.DetachDeleteInternetGateway, input.Region, vpcID).Get(ctx, nil); err != nil {
		return err
	}

	if err := workflow.ExecuteActivity(ctx, aws.DeleteVPC, input.Region, vpcID).Get(ctx, nil); err != nil {
		return err
	}

	return nil
}
