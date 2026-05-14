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

	if err := workflow.ExecuteActivity(ctx, aws.CreateEKSCluster, activities.CreateEKSClusterInput{
		Region:      input.Region,
		ClusterName: input.ClusterName,
		VpcID:       input.VpcID,
		SubnetIDs:   input.SubnetIDs,
		Environment: input.Environment,
		Team:        input.Team,
	}).Get(ctx, nil); err != nil {
		return err
	}

	return workflow.ExecuteActivity(ctx, aws.CreateNodeGroup, activities.CreateNodeGroupInput{
		Region:       input.Region,
		ClusterName:  input.ClusterName,
		SubnetIDs:    input.SubnetIDs,
		NodeCount:    input.NodeCount,
		InstanceType: input.NodeInstanceType,
		Environment:  input.Environment,
		Team:         input.Team,
	}).Get(ctx, nil)
}

func SpinDownEKSWorkflow(ctx workflow.Context, input SpinDownEKSInput) error {
	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	aws := &activities.AWSActivities{}

	if err := workflow.ExecuteActivity(ctx, aws.DeleteNodeGroup, activities.DeleteNodeGroupInput{
		Region:      input.Region,
		ClusterName: input.ClusterName,
	}).Get(ctx, nil); err != nil {
		return err
	}

	return workflow.ExecuteActivity(ctx, aws.DeleteEKSCluster, activities.DeleteEKSClusterInput{
		Region:      input.Region,
		ClusterName: input.ClusterName,
	}).Get(ctx, nil)
}
