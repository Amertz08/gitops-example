package workflows

import (
	"github.com/Amertz08/gitops-example/internal/activities"
	"go.temporal.io/sdk/workflow"
)

type SpinUpNetworkInput struct {
	Region      string
	Environment string
	Team        string
}

type SpinUpNetworkOutput struct {
	VpcID     string
	SubnetIDs []string
}

type SpinDownNetworkInput struct {
	Region string
	VpcID  string
}

func SpinUpNetworkWorkflow(ctx workflow.Context, input SpinUpNetworkInput) (SpinUpNetworkOutput, error) {
	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	aws := &activities.AWSActivities{}

	var vpcID string
	if err := workflow.ExecuteActivity(ctx, aws.CreateVPC, input.Region, input.Environment, input.Team).
		Get(ctx, &vpcID); err != nil {
		return SpinUpNetworkOutput{}, err
	}

	var subnetIDs []string
	if err := workflow.ExecuteActivity(ctx, aws.CreateSubnets, input.Region, vpcID, input.Environment, input.Team).
		Get(ctx, &subnetIDs); err != nil {
		return SpinUpNetworkOutput{}, err
	}

	if err := workflow.ExecuteActivity(ctx, aws.CreateInternetGateway, input.Region, vpcID, input.Environment, input.Team).
		Get(ctx, nil); err != nil {
		return SpinUpNetworkOutput{}, err
	}

	if err := workflow.ExecuteActivity(ctx, aws.ConfigureRouteTables, input.Region, vpcID, subnetIDs, input.Environment, input.Team).
		Get(ctx, nil); err != nil {
		return SpinUpNetworkOutput{}, err
	}

	return SpinUpNetworkOutput{VpcID: vpcID, SubnetIDs: subnetIDs}, nil
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
