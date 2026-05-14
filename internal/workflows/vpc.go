package workflows

import (
	"fmt"

	"github.com/Amertz08/gitops-example/internal/activities"
	"go.temporal.io/sdk/workflow"
)

type SpinUpNetworkInput struct {
	Region      string
	VpcCIDR     string
	Subnets     []activities.SubnetConfig
	Environment string
	Team        string
}

func (i SpinUpNetworkInput) validate() error {
	switch {
	case i.Region == "":
		return fmt.Errorf("Region is required")
	case i.Environment == "":
		return fmt.Errorf("Environment is required")
	case i.Team == "":
		return fmt.Errorf("Team is required")
	}
	return nil
}

type SpinUpNetworkOutput struct {
	VpcID     string
	SubnetIDs []string
}

type SpinDownNetworkInput struct {
	Region string
	VpcID  string
}

func (i SpinDownNetworkInput) validate() error {
	switch {
	case i.Region == "":
		return fmt.Errorf("Region is required")
	case i.VpcID == "":
		return fmt.Errorf("VpcID is required")
	}
	return nil
}

func SpinUpNetworkWorkflow(
	ctx workflow.Context,
	input SpinUpNetworkInput,
) (output SpinUpNetworkOutput, err error) {
	if err = invalidInput(input); err != nil {
		return
	}

	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	aws := &activities.AWSActivities{}
	logger := workflow.GetLogger(ctx)

	var s Saga
	defer func() {
		if err != nil {
			s.Compensate(ctx)
		}
	}()

	var vpcID string
	if err = workflow.ExecuteActivity(ctx, aws.CreateVPC, activities.CreateVPCInput{
		Region:      input.Region,
		VpcCIDR:     input.VpcCIDR,
		Environment: input.Environment,
		Team:        input.Team,
	}).Get(ctx, &vpcID); err != nil {
		return
	}
	logger.Info("VPC created", "vpcID", vpcID)
	s.AddCompensation(func(cctx workflow.Context) {
		if err := workflow.ExecuteActivity(cctx, aws.DeleteVPC, activities.DeleteVPCInput{
			Region: input.Region,
			VpcID:  vpcID,
		}).Get(cctx, nil); err != nil {
			logger.Error("saga: failed to delete VPC", "vpcID", vpcID, "error", err)
		}
	})

	var subnetIDs []string
	if err = workflow.ExecuteActivity(ctx, aws.CreateSubnets, activities.CreateSubnetsInput{
		Region:      input.Region,
		VpcID:       vpcID,
		Subnets:     input.Subnets,
		Environment: input.Environment,
		Team:        input.Team,
	}).Get(ctx, &subnetIDs); err != nil {
		return
	}
	logger.Info("subnets created", "vpcID", vpcID, "count", len(subnetIDs))
	s.AddCompensation(func(cctx workflow.Context) {
		if err := workflow.ExecuteActivity(cctx, aws.DeleteSubnets, activities.DeleteSubnetsInput{
			Region: input.Region,
			VpcID:  vpcID,
		}).Get(cctx, nil); err != nil {
			logger.Error("saga: failed to delete subnets", "vpcID", vpcID, "error", err)
		}
	})

	var igwID string
	if err = workflow.ExecuteActivity(ctx, aws.CreateInternetGateway, activities.CreateInternetGatewayInput{
		Region:      input.Region,
		VpcID:       vpcID,
		Environment: input.Environment,
		Team:        input.Team,
	}).
		Get(ctx, &igwID); err != nil {
		return
	}
	logger.Info("internet gateway created and attached", "vpcID", vpcID, "igwID", igwID)
	s.AddCompensation(func(cctx workflow.Context) {
		if err := workflow.ExecuteActivity(cctx, aws.DetachDeleteInternetGateway, activities.DetachDeleteInternetGatewayInput{
			Region: input.Region,
			VpcID:  vpcID,
		}).
			Get(cctx, nil); err != nil {
			logger.Error("saga: failed to detach/delete IGW", "vpcID", vpcID, "error", err)
		}
	})

	if err = workflow.ExecuteActivity(ctx, aws.ConfigureRouteTables, activities.ConfigureRouteTablesInput{
		Region:      input.Region,
		VpcID:       vpcID,
		IgwID:       igwID,
		SubnetIDs:   subnetIDs,
		Environment: input.Environment,
		Team:        input.Team,
	}).
		Get(ctx, nil); err != nil {
		return
	}
	logger.Info("route tables configured", "vpcID", vpcID)
	s.AddCompensation(func(cctx workflow.Context) {
		if err := workflow.ExecuteActivity(cctx, aws.DeleteRouteTables, activities.DeleteRouteTablesInput{
			Region: input.Region,
			VpcID:  vpcID,
		}).
			Get(cctx, nil); err != nil {
			logger.Error("saga: failed to delete route tables", "vpcID", vpcID, "error", err)
		}
	})

	output = SpinUpNetworkOutput{VpcID: vpcID, SubnetIDs: subnetIDs}
	return
}

func SpinDownNetworkWorkflow(ctx workflow.Context, input SpinDownNetworkInput) error {
	if err := invalidInput(input); err != nil {
		return err
	}

	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	aws := &activities.AWSActivities{}
	logger := workflow.GetLogger(ctx)

	if err := workflow.ExecuteActivity(ctx, aws.DeleteSubnets, activities.DeleteSubnetsInput{
		Region: input.Region,
		VpcID:  input.VpcID,
	}).Get(ctx, nil); err != nil {
		return err
	}
	logger.Info("subnets deleted", "vpcID", input.VpcID)

	if err := workflow.ExecuteActivity(ctx, aws.DeleteRouteTables, activities.DeleteRouteTablesInput{
		Region: input.Region,
		VpcID:  input.VpcID,
	}).
		Get(ctx, nil); err != nil {
		return err
	}
	logger.Info("route tables deleted", "vpcID", input.VpcID)

	if err := workflow.ExecuteActivity(ctx, aws.DetachDeleteInternetGateway, activities.DetachDeleteInternetGatewayInput{
		Region: input.Region,
		VpcID:  input.VpcID,
	}).
		Get(ctx, nil); err != nil {
		return err
	}
	logger.Info("internet gateway detached and deleted", "vpcID", input.VpcID)

	if err := workflow.ExecuteActivity(ctx, aws.DeleteVPC, activities.DeleteVPCInput{
		Region: input.Region,
		VpcID:  input.VpcID,
	}).Get(ctx, nil); err != nil {
		return err
	}
	logger.Info("VPC deleted", "vpcID", input.VpcID)
	return nil
}
