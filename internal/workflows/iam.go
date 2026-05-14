package workflows

import (
	"fmt"

	"github.com/Amertz08/gitops-example/internal/activities"
	"go.temporal.io/sdk/workflow"
)

const eksTrustPolicy = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"eks.amazonaws.com"},"Action":"sts:AssumeRole"}]}`

const ec2TrustPolicy = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}`

type SpinUpIAMInput struct {
	Region      string
	ClusterName string
	Environment string
	Team        string
}

type SpinUpIAMOutput struct {
	ClusterRoleARN  string
	ClusterRoleName string
	NodeRoleARN     string
	NodeRoleName    string
}

type SpinDownIAMInput struct {
	ClusterRoleName string
	NodeRoleName    string
}

func SpinUpIAMWorkflow(
	ctx workflow.Context,
	input SpinUpIAMInput,
) (output SpinUpIAMOutput, err error) {
	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	aws := &activities.AWSActivities{}
	logger := workflow.GetLogger(ctx)

	var s Saga
	defer func() {
		if err != nil {
			s.Compensate(ctx)
		}
	}()

	clusterRoleName := fmt.Sprintf("%s-eks-cluster-role", input.ClusterName)
	var clusterRoleARN string
	if err = workflow.ExecuteActivity(ctx, aws.CreateIAMRole, activities.CreateIAMRoleInput{
		RoleName:    clusterRoleName,
		Description: fmt.Sprintf("EKS cluster role for %s", input.ClusterName),
		TrustPolicy: eksTrustPolicy,
		PolicyARNs:  []string{"arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"},
		Environment: input.Environment,
		Team:        input.Team,
	}).Get(ctx, &clusterRoleARN); err != nil {
		return
	}
	logger.Info("cluster IAM role created", "roleName", clusterRoleName)
	s.AddCompensation(func(cctx workflow.Context) {
		if err := workflow.ExecuteActivity(cctx, aws.DeleteIAMRole, activities.DeleteIAMRoleInput{
			RoleName: clusterRoleName,
		}).Get(cctx, nil); err != nil {
			logger.Error(
				"saga: failed to delete cluster IAM role",
				"roleName",
				clusterRoleName,
				"error",
				err,
			)
		}
	})

	nodeRoleName := fmt.Sprintf("%s-eks-node-role", input.ClusterName)
	var nodeRoleARN string
	if err = workflow.ExecuteActivity(ctx, aws.CreateIAMRole, activities.CreateIAMRoleInput{
		RoleName:    nodeRoleName,
		Description: fmt.Sprintf("EKS node role for %s", input.ClusterName),
		TrustPolicy: ec2TrustPolicy,
		PolicyARNs: []string{
			"arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
			"arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
			"arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
		},
		Environment: input.Environment,
		Team:        input.Team,
	}).Get(ctx, &nodeRoleARN); err != nil {
		return
	}
	logger.Info("node IAM role created", "roleName", nodeRoleName)
	s.AddCompensation(func(cctx workflow.Context) {
		if err := workflow.ExecuteActivity(cctx, aws.DeleteIAMRole, activities.DeleteIAMRoleInput{
			RoleName: nodeRoleName,
		}).Get(cctx, nil); err != nil {
			logger.Error(
				"saga: failed to delete node IAM role",
				"roleName",
				nodeRoleName,
				"error",
				err,
			)
		}
	})

	output = SpinUpIAMOutput{
		ClusterRoleARN:  clusterRoleARN,
		ClusterRoleName: clusterRoleName,
		NodeRoleARN:     nodeRoleARN,
		NodeRoleName:    nodeRoleName,
	}
	return
}

func SpinDownIAMWorkflow(ctx workflow.Context, input SpinDownIAMInput) error {
	ctx = workflow.WithActivityOptions(ctx, activityOptions)
	aws := &activities.AWSActivities{}
	logger := workflow.GetLogger(ctx)

	if err := workflow.ExecuteActivity(ctx, aws.DeleteIAMRole, activities.DeleteIAMRoleInput{
		RoleName: input.ClusterRoleName,
	}).Get(ctx, nil); err != nil {
		return err
	}
	logger.Info("cluster IAM role deleted", "roleName", input.ClusterRoleName)

	if err := workflow.ExecuteActivity(ctx, aws.DeleteIAMRole, activities.DeleteIAMRoleInput{
		RoleName: input.NodeRoleName,
	}).Get(ctx, nil); err != nil {
		return err
	}
	logger.Info("node IAM role deleted", "roleName", input.NodeRoleName)

	return nil
}
