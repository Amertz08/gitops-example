package workflows

import (
	"github.com/Amertz08/gitops-example/internal/activities"
	"go.temporal.io/sdk/workflow"
)

type SpinUpEKSInput struct {
	Region           string
	ClusterName      string
	VpcID            string
	SubnetIDs        []string
	NodeCount        int32
	NodeInstanceType string
	Environment      string
	Team             string
}

type SpinDownEKSInput struct {
	Region      string
	ClusterName string
}

func SpinUpEKSWorkflow(ctx workflow.Context, input SpinUpEKSInput) error {
	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	aws := &activities.AWSActivities{}

	if err := workflow.ExecuteActivity(ctx, aws.CreateEKSCluster, input.Region, input.ClusterName, input.VpcID, input.SubnetIDs, input.Environment, input.Team).
		Get(ctx, nil); err != nil {
		return err
	}

	return workflow.ExecuteActivity(ctx, aws.CreateNodeGroup, input.Region, input.ClusterName, input.SubnetIDs, input.NodeCount, input.NodeInstanceType, input.Environment, input.Team).
		Get(ctx, nil)
}

func SpinDownEKSWorkflow(ctx workflow.Context, input SpinDownEKSInput) (string, error) {
	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	aws := &activities.AWSActivities{}

	if err := workflow.ExecuteActivity(ctx, aws.DeleteNodeGroup, input.Region, input.ClusterName).
		Get(ctx, nil); err != nil {
		return "", err
	}

	var vpcID string
	if err := workflow.ExecuteActivity(ctx, aws.DeleteEKSCluster, input.Region, input.ClusterName).
		Get(ctx, &vpcID); err != nil {
		return "", err
	}

	return vpcID, nil
}
