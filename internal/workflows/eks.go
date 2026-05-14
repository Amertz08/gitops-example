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

func SpinUpEKSWorkflow(ctx workflow.Context, input SpinUpEKSInput) (err error) {
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
