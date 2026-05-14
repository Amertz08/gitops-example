package workflows

import (
	"fmt"
	"strings"
	"time"

	"github.com/Amertz08/gitops-example/internal/activities"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type SpinUpInput struct {
	Region           string
	ClusterName      string
	ClusterRoleARN   string
	NodeRoleARN      string
	NodeCount        int32
	NodeInstanceType string
	Environment      string
	Team             string
	VpcCIDR          string
	Subnets          []activities.SubnetConfig
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
	case i.ClusterRoleARN == "":
		return fmt.Errorf("ClusterRoleARN is required")
	case i.NodeRoleARN == "":
		return fmt.Errorf("NodeRoleARN is required")
	case i.NodeCount <= 0:
		return fmt.Errorf("NodeCount must be greater than 0")
	}
	for idx, sc := range i.Subnets {
		if strings.TrimSpace(sc.CIDR) == "" {
			return fmt.Errorf("Subnets[%d].CIDR is required", idx)
		}
	}
	return nil
}

func SpinUpWorkflow(ctx workflow.Context, input SpinUpInput) (err error) {
	if valErr := input.validate(); valErr != nil {
		return temporal.NewNonRetryableApplicationError(valErr.Error(), "InvalidInput", valErr)
	}

	logger := workflow.GetLogger(ctx)
	var s Saga
	defer func() {
		if err != nil {
			s.Compensate(ctx)
		}
	}()

	var network SpinUpNetworkOutput
	if err = workflow.ExecuteChildWorkflow(ctx, SpinUpNetworkWorkflow, SpinUpNetworkInput{
		Region:      input.Region,
		VpcCIDR:     input.VpcCIDR,
		Subnets:     input.Subnets,
		Environment: input.Environment,
		Team:        input.Team,
	}).Get(ctx, &network); err != nil {
		return
	}
	logger.Info("network ready", "vpcID", network.VpcID, "subnetCount", len(network.SubnetIDs))
	s.AddCompensation(func(cctx workflow.Context) {
		if err := workflow.ExecuteChildWorkflow(cctx, SpinDownNetworkWorkflow, SpinDownNetworkInput{
			Region: input.Region,
			VpcID:  network.VpcID,
		}).Get(cctx, nil); err != nil {
			logger.Error("saga: failed to spin down network", "vpcID", network.VpcID, "error", err)
		}
	})

	if err = workflow.ExecuteChildWorkflow(ctx, SpinUpEKSWorkflow, SpinUpEKSInput{
		Region:           input.Region,
		ClusterName:      input.ClusterName,
		ClusterRoleARN:   input.ClusterRoleARN,
		NodeRoleARN:      input.NodeRoleARN,
		VpcID:            network.VpcID,
		SubnetIDs:        network.SubnetIDs,
		NodeCount:        input.NodeCount,
		NodeInstanceType: input.NodeInstanceType,
		Environment:      input.Environment,
		Team:             input.Team,
	}).Get(ctx, nil); err != nil {
		return
	}
	logger.Info("spin up complete", "clusterName", input.ClusterName, "vpcID", network.VpcID)
	return
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

	logger := workflow.GetLogger(ctx)

	if err := workflow.ExecuteChildWorkflow(ctx, SpinDownEKSWorkflow, SpinDownEKSInput{
		Region:      input.Region,
		ClusterName: input.ClusterName,
	}).Get(ctx, nil); err != nil {
		return err
	}
	logger.Info("EKS torn down", "clusterName", input.ClusterName)

	if err := workflow.ExecuteChildWorkflow(ctx, SpinDownNetworkWorkflow, SpinDownNetworkInput{
		Region: input.Region,
		VpcID:  input.VpcID,
	}).Get(ctx, nil); err != nil {
		return err
	}
	logger.Info("spin down complete", "clusterName", input.ClusterName, "vpcID", input.VpcID)
	return nil
}
