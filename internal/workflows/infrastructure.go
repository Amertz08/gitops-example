package workflows

import (
	"time"

	"github.com/Amertz08/gitops-example/internal/activities"
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

type SpinUpNetworkInput struct {
	Region string
}

type SpinUpNetworkOutput struct {
	VpcID     string
	SubnetIDs []string
}

type SpinUpEKSInput struct {
	Region           string
	ClusterName      string
	VpcID            string
	SubnetIDs        []string
	NodeCount        int32
	NodeInstanceType string
}

type SpinDownNetworkInput struct {
	Region string
	VpcID  string
}

type SpinDownEKSInput struct {
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

func SpinUpNetworkWorkflow(ctx workflow.Context, input SpinUpNetworkInput) (SpinUpNetworkOutput, error) {
	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	aws := &activities.AWSActivities{}

	var vpcID string
	if err := workflow.ExecuteActivity(ctx, aws.CreateVPC, input.Region).
		Get(ctx, &vpcID); err != nil {
		return SpinUpNetworkOutput{}, err
	}

	var subnetIDs []string
	if err := workflow.ExecuteActivity(ctx, aws.CreateSubnets, input.Region, vpcID).
		Get(ctx, &subnetIDs); err != nil {
		return SpinUpNetworkOutput{}, err
	}

	if err := workflow.ExecuteActivity(ctx, aws.CreateInternetGateway, input.Region, vpcID).
		Get(ctx, nil); err != nil {
		return SpinUpNetworkOutput{}, err
	}

	if err := workflow.ExecuteActivity(ctx, aws.ConfigureRouteTables, input.Region, vpcID, subnetIDs).
		Get(ctx, nil); err != nil {
		return SpinUpNetworkOutput{}, err
	}

	return SpinUpNetworkOutput{VpcID: vpcID, SubnetIDs: subnetIDs}, nil
}

func SpinUpEKSWorkflow(ctx workflow.Context, input SpinUpEKSInput) error {
	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	aws := &activities.AWSActivities{}

	if err := workflow.ExecuteActivity(ctx, aws.CreateEKSCluster, input.Region, input.ClusterName, input.VpcID, input.SubnetIDs).
		Get(ctx, nil); err != nil {
		return err
	}

	return workflow.ExecuteActivity(ctx, aws.CreateNodeGroup, input.Region, input.ClusterName, input.SubnetIDs, input.NodeCount, input.NodeInstanceType).
		Get(ctx, nil)
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

func SpinDownNetworkWorkflow(ctx workflow.Context, input SpinDownNetworkInput) error {
	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	aws := &activities.AWSActivities{}

	if err := workflow.ExecuteActivity(ctx, aws.DeleteSubnets, input.Region, input.VpcID).
		Get(ctx, nil); err != nil {
		return err
	}

	if err := workflow.ExecuteActivity(ctx, aws.DetachDeleteInternetGateway, input.Region, input.VpcID).
		Get(ctx, nil); err != nil {
		return err
	}

	return workflow.ExecuteActivity(ctx, aws.DeleteVPC, input.Region, input.VpcID).
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
