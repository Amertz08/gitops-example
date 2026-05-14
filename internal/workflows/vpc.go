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
	if err := workflow.ExecuteActivity(ctx, aws.CreateVPC, activities.CreateVPCInput{
		Region:      input.Region,
		Environment: input.Environment,
		Team:        input.Team,
	}).Get(ctx, &vpcID); err != nil {
		return SpinUpNetworkOutput{}, err
	}

	var subnetIDs []string
	if err := workflow.ExecuteActivity(ctx, aws.CreateSubnets, activities.CreateSubnetsInput{
		Region:      input.Region,
		VpcID:       vpcID,
		Environment: input.Environment,
		Team:        input.Team,
	}).Get(ctx, &subnetIDs); err != nil {
		return SpinUpNetworkOutput{}, err
	}

	var igwID string
	if err := workflow.ExecuteActivity(ctx, aws.CreateInternetGateway, activities.CreateInternetGatewayInput{
		Region:      input.Region,
		VpcID:       vpcID,
		Environment: input.Environment,
		Team:        input.Team,
	}).Get(ctx, &igwID); err != nil {
		return SpinUpNetworkOutput{}, err
	}

	if err := workflow.ExecuteActivity(ctx, aws.ConfigureRouteTables, activities.ConfigureRouteTablesInput{
		Region:      input.Region,
		VpcID:       vpcID,
		IgwID:       igwID,
		SubnetIDs:   subnetIDs,
		Environment: input.Environment,
		Team:        input.Team,
	}).Get(ctx, nil); err != nil {
		return SpinUpNetworkOutput{}, err
	}

	return SpinUpNetworkOutput{VpcID: vpcID, SubnetIDs: subnetIDs}, nil
}

func SpinDownNetworkWorkflow(ctx workflow.Context, input SpinDownNetworkInput) error {
	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	aws := &activities.AWSActivities{}

	if err := workflow.ExecuteActivity(ctx, aws.DeleteSubnets, activities.DeleteSubnetsInput{
		Region: input.Region,
		VpcID:  input.VpcID,
	}).Get(ctx, nil); err != nil {
		return err
	}

	if err := workflow.ExecuteActivity(ctx, aws.DeleteRouteTables, activities.DeleteRouteTablesInput{
		Region: input.Region,
		VpcID:  input.VpcID,
	}).Get(ctx, nil); err != nil {
		return err
	}

	if err := workflow.ExecuteActivity(ctx, aws.DetachDeleteInternetGateway, activities.DetachDeleteInternetGatewayInput{
		Region: input.Region,
		VpcID:  input.VpcID,
	}).Get(ctx, nil); err != nil {
		return err
	}

	return workflow.ExecuteActivity(ctx, aws.DeleteVPC, activities.DeleteVPCInput{
		Region: input.Region,
		VpcID:  input.VpcID,
	}).Get(ctx, nil)
}
