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
	case (i.ClusterRoleARN == "") != (i.NodeRoleARN == ""):
		return fmt.Errorf("ClusterRoleARN and NodeRoleARN must both be provided or both be empty")
	}
	for idx, sc := range i.Subnets {
		if strings.TrimSpace(sc.CIDR) == "" {
			return fmt.Errorf("Subnets[%d].CIDR is required", idx)
		}
	}
	return nil
}

type SpinDownInput struct {
	Region          string
	ClusterName     string
	VpcID           string
	ClusterRoleName string
	NodeRoleName    string
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

var activityOptions = workflow.ActivityOptions{
	StartToCloseTimeout: 30 * time.Minute,
	RetryPolicy: &temporal.RetryPolicy{
		MaximumAttempts: 3,
		InitialInterval: 10 * time.Second,
	},
}

func SpinUpWorkflow(ctx workflow.Context, input SpinUpInput) (err error) {
	if err = invalidInput(input); err != nil {
		return
	}

	logger := workflow.GetLogger(ctx)
	var s Saga
	defer func() {
		if err != nil {
			s.Compensate(ctx)
		}
	}()

	clusterRoleARN := input.ClusterRoleARN
	nodeRoleARN := input.NodeRoleARN

	// Start network unconditionally; start IAM concurrently when role ARNs are not pre-supplied.
	networkFuture := workflow.ExecuteChildWorkflow(ctx, SpinUpNetworkWorkflow, SpinUpNetworkInput{
		Region:      input.Region,
		VpcCIDR:     input.VpcCIDR,
		Subnets:     input.Subnets,
		Environment: input.Environment,
		Team:        input.Team,
	})

	var iamFuture workflow.ChildWorkflowFuture
	if clusterRoleARN == "" {
		iamFuture = workflow.ExecuteChildWorkflow(ctx, SpinUpIAMWorkflow, SpinUpEKSIAMInput{
			Region:      input.Region,
			ClusterName: input.ClusterName,
			Environment: input.Environment,
			Team:        input.Team,
		})
	}

	// Collect IAM result (if started).
	var iamErr error
	if iamFuture != nil {
		var iamOut SpinUpEKSIAMOutput
		iamErr = iamFuture.Get(ctx, &iamOut)
		if iamErr == nil {
			clusterRoleARN = iamOut.ClusterRoleARN
			nodeRoleARN = iamOut.NodeRoleARN
			logger.Info(
				"IAM roles created",
				"clusterRole",
				iamOut.ClusterRoleName,
				"nodeRole",
				iamOut.NodeRoleName,
			)
			s.AddCompensation(func(cctx workflow.Context) {
				if err := workflow.ExecuteChildWorkflow(
					cctx,
					SpinDownIAMWorkflow,
					SpinDownEKSIAMInput{
						ClusterRoleName: iamOut.ClusterRoleName,
						NodeRoleName:    iamOut.NodeRoleName,
					},
				).Get(cctx, nil); err != nil {
					logger.Error("saga: failed to spin down IAM roles", "error", err)
				}
			})
		}
	}

	// Always wait for network before returning — prevents orphaned resources if IAM failed.
	var network SpinUpNetworkOutput
	netErr := networkFuture.Get(ctx, &network)
	if netErr == nil {
		logger.Info("network ready", "vpcID", network.VpcID, "subnetCount", len(network.SubnetIDs))
		s.AddCompensation(func(cctx workflow.Context) {
			if err := workflow.ExecuteChildWorkflow(cctx, SpinDownNetworkWorkflow, SpinDownNetworkInput{
				Region: input.Region,
				VpcID:  network.VpcID,
			}).
				Get(cctx, nil); err != nil {
				logger.Error(
					"saga: failed to spin down network",
					"vpcID",
					network.VpcID,
					"error",
					err,
				)
			}
		})
	}

	if iamErr != nil {
		err = iamErr
		return
	}
	if netErr != nil {
		err = netErr
		return
	}

	if err = workflow.ExecuteChildWorkflow(ctx, SpinUpEKSWorkflow, SpinUpEKSInput{
		Region:           input.Region,
		ClusterName:      input.ClusterName,
		ClusterRoleARN:   clusterRoleARN,
		NodeRoleARN:      nodeRoleARN,
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

func SpinDownWorkflow(ctx workflow.Context, input SpinDownInput) error {
	if err := invalidInput(input); err != nil {
		return err
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
	logger.Info("network torn down", "vpcID", input.VpcID)

	if input.ClusterRoleName != "" || input.NodeRoleName != "" {
		if err := workflow.ExecuteChildWorkflow(ctx, SpinDownIAMWorkflow, SpinDownEKSIAMInput{
			ClusterRoleName: input.ClusterRoleName,
			NodeRoleName:    input.NodeRoleName,
		}).Get(ctx, nil); err != nil {
			return err
		}
		logger.Info("IAM roles deleted")
	}

	logger.Info("spin down complete", "clusterName", input.ClusterName, "vpcID", input.VpcID)
	return nil
}
