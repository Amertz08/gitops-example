package workflows

import (
	"fmt"

	"github.com/Amertz08/gitops-example/internal/activities"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type SpinUpEKSInput struct {
	Region           string
	ClusterName      string
	ClusterRoleARN   string
	NodeRoleARN      string
	VpcID            string
	SubnetIDs        []string
	NodeCount        int32
	NodeInstanceType string
	Environment      string
	Team             string
}

func (i SpinUpEKSInput) validate() error {
	switch {
	case i.Region == "":
		return fmt.Errorf("Region is required")
	case i.ClusterName == "":
		return fmt.Errorf("ClusterName is required")
	case i.ClusterRoleARN == "":
		return fmt.Errorf("ClusterRoleARN is required")
	case i.NodeRoleARN == "":
		return fmt.Errorf("NodeRoleARN is required")
	case i.VpcID == "":
		return fmt.Errorf("VpcID is required")
	case len(i.SubnetIDs) == 0:
		return fmt.Errorf("SubnetIDs is required")
	case i.NodeInstanceType == "":
		return fmt.Errorf("NodeInstanceType is required")
	case i.Environment == "":
		return fmt.Errorf("Environment is required")
	case i.Team == "":
		return fmt.Errorf("Team is required")
	case i.NodeCount <= 0:
		return fmt.Errorf("NodeCount must be greater than 0")
	}
	return nil
}

type SpinDownEKSInput struct {
	Region      string
	ClusterName string
}

func (i SpinDownEKSInput) validate() error {
	switch {
	case i.Region == "":
		return fmt.Errorf("Region is required")
	case i.ClusterName == "":
		return fmt.Errorf("ClusterName is required")
	}
	return nil
}

func SpinUpEKSWorkflow(ctx workflow.Context, input SpinUpEKSInput) (err error) {
	if valErr := input.validate(); valErr != nil {
		return temporal.NewNonRetryableApplicationError(valErr.Error(), "InvalidInput", valErr)
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

	if err = workflow.ExecuteActivity(ctx, aws.CreateEKSCluster, activities.CreateEKSClusterInput{
		Region:      input.Region,
		ClusterName: input.ClusterName,
		RoleARN:     input.ClusterRoleARN,
		VpcID:       input.VpcID,
		SubnetIDs:   input.SubnetIDs,
		Environment: input.Environment,
		Team:        input.Team,
	}).Get(ctx, nil); err != nil {
		return
	}
	logger.Info("EKS cluster active", "clusterName", input.ClusterName)
	s.AddCompensation(func(cctx workflow.Context) {
		if err := workflow.ExecuteActivity(cctx, aws.DeleteEKSCluster, activities.DeleteEKSClusterInput{
			Region:      input.Region,
			ClusterName: input.ClusterName,
		}).
			Get(cctx, nil); err != nil {
			logger.Error(
				"saga: failed to delete EKS cluster",
				"clusterName",
				input.ClusterName,
				"error",
				err,
			)
		}
	})

	if err = workflow.ExecuteActivity(ctx, aws.CreateNodeGroup, activities.CreateNodeGroupInput{
		Region:       input.Region,
		ClusterName:  input.ClusterName,
		NodeRoleARN:  input.NodeRoleARN,
		SubnetIDs:    input.SubnetIDs,
		NodeCount:    input.NodeCount,
		InstanceType: input.NodeInstanceType,
		Environment:  input.Environment,
		Team:         input.Team,
	}).Get(ctx, nil); err != nil {
		return
	}
	logger.Info(
		"node group created",
		"clusterName",
		input.ClusterName,
		"nodeCount",
		input.NodeCount,
	)
	return
}

func SpinDownEKSWorkflow(ctx workflow.Context, input SpinDownEKSInput) error {
	if err := input.validate(); err != nil {
		return temporal.NewNonRetryableApplicationError(err.Error(), "InvalidInput", err)
	}

	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	aws := &activities.AWSActivities{}
	logger := workflow.GetLogger(ctx)

	if err := workflow.ExecuteActivity(ctx, aws.DeleteNodeGroup, activities.DeleteNodeGroupInput{
		Region:      input.Region,
		ClusterName: input.ClusterName,
	}).Get(ctx, nil); err != nil {
		return err
	}
	logger.Info("node group deleted", "clusterName", input.ClusterName)

	if err := workflow.ExecuteActivity(ctx, aws.DeleteEKSCluster, activities.DeleteEKSClusterInput{
		Region:      input.Region,
		ClusterName: input.ClusterName,
	}).Get(ctx, nil); err != nil {
		return err
	}
	logger.Info("EKS cluster deleted", "clusterName", input.ClusterName)
	return nil
}
