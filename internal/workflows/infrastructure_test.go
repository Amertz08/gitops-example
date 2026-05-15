package workflows

import (
	"errors"
	"testing"

	"github.com/Amertz08/gitops-example/internal/activities"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
)

type InfraWorkflowTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
	env *testsuite.TestWorkflowEnvironment
}

func (s *InfraWorkflowTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
	s.env.RegisterWorkflow(SpinUpNetworkWorkflow)
	s.env.RegisterWorkflow(SpinUpIAMWorkflow)
	s.env.RegisterWorkflow(SpinUpEKSWorkflow)
	s.env.RegisterWorkflow(SpinDownEKSWorkflow)
	s.env.RegisterWorkflow(SpinDownNetworkWorkflow)
	s.env.RegisterWorkflow(SpinDownIAMWorkflow)
}

func (s *InfraWorkflowTestSuite) AfterTest(_, _ string) {
	s.env.AssertExpectations(s.T())
}

func TestInfraWorkflowSuite(t *testing.T) {
	suite.Run(t, new(InfraWorkflowTestSuite))
}

// mockHappyPathNetworkActivities mocks all network activities to return success.
func (s *InfraWorkflowTestSuite) mockHappyPathNetworkActivities() {
	aws := &activities.AWSActivities{}
	s.env.OnActivity(aws.CreateVPC, mock.Anything, mock.Anything).Return("vpc-123", nil)
	s.env.OnActivity(aws.CreateSubnets, mock.Anything, mock.Anything).
		Return([]string{"subnet-a", "subnet-b"}, nil)
	s.env.OnActivity(aws.CreateInternetGateway, mock.Anything, mock.Anything).Return("igw-123", nil)
	s.env.OnActivity(aws.ConfigureRouteTables, mock.Anything, mock.Anything).Return(nil)
}

// mockHappyPathEKSActivities mocks all EKS activities to return success.
func (s *InfraWorkflowTestSuite) mockHappyPathEKSActivities() {
	aws := &activities.AWSActivities{}
	s.env.OnActivity(aws.CreateEKSCluster, mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity(aws.CreateNodeGroup, mock.Anything, mock.Anything).Return(nil)
}

func validSpinUpInput() SpinUpInput {
	return SpinUpInput{
		Region:           "us-east-1",
		ClusterName:      "my-cluster",
		NodeCount:        2,
		NodeInstanceType: "t3.medium",
		Environment:      "prod",
		Team:             "platform",
	}
}

func (s *InfraWorkflowTestSuite) Test_SpinUp_InvalidInput() {
	s.env.ExecuteWorkflow(SpinUpWorkflow, SpinUpInput{})

	s.True(s.env.IsWorkflowCompleted())
	err := s.env.GetWorkflowError()
	s.Error(err)
	var appErr *temporal.ApplicationError
	s.True(errors.As(err, &appErr))
	s.Equal("InvalidInput", appErr.Type())
}

// When no role ARNs are supplied, SpinUpWorkflow must run SpinUpIAMWorkflow concurrently with
// SpinUpNetworkWorkflow, then use the resulting ARNs for SpinUpEKSWorkflow.
func (s *InfraWorkflowTestSuite) Test_SpinUp_Success_WithoutPreSuppliedARNs() {
	aws := &activities.AWSActivities{}
	s.env.OnActivity(aws.CreateIAMRole, mock.Anything, mock.Anything).
		Return("arn:aws:iam::123:role/cluster-role", nil).Once()
	s.env.OnActivity(aws.CreateIAMRole, mock.Anything, mock.Anything).
		Return("arn:aws:iam::123:role/node-role", nil).Once()
	s.mockHappyPathNetworkActivities()
	s.mockHappyPathEKSActivities()

	s.env.ExecuteWorkflow(SpinUpWorkflow, validSpinUpInput())

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

// When role ARNs are pre-supplied, SpinUpWorkflow must skip IAM creation entirely.
func (s *InfraWorkflowTestSuite) Test_SpinUp_Success_WithPreSuppliedARNs() {
	s.mockHappyPathNetworkActivities()
	s.mockHappyPathEKSActivities()

	input := validSpinUpInput()
	input.ClusterRoleARN = "arn:aws:iam::123:role/existing-cluster-role"
	input.NodeRoleARN = "arn:aws:iam::123:role/existing-node-role"
	s.env.ExecuteWorkflow(SpinUpWorkflow, input)

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

func (s *InfraWorkflowTestSuite) Test_SpinDown_InvalidInput() {
	s.env.ExecuteWorkflow(SpinDownWorkflow, SpinDownInput{})

	s.True(s.env.IsWorkflowCompleted())
	err := s.env.GetWorkflowError()
	s.Error(err)
	var appErr *temporal.ApplicationError
	s.True(errors.As(err, &appErr))
	s.Equal("InvalidInput", appErr.Type())
}

// SpinDownWorkflow tears down EKS, network, and (when role names are provided) IAM.
func (s *InfraWorkflowTestSuite) Test_SpinDown_Success_WithRoles() {
	aws := &activities.AWSActivities{}
	s.env.OnActivity(aws.DeleteNodeGroup, mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity(aws.DeleteEKSCluster, mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity(aws.DeleteSubnets, mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity(aws.DeleteRouteTables, mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity(aws.DetachDeleteInternetGateway, mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity(aws.DeleteVPC, mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity(aws.DeleteIAMRole, mock.Anything, mock.Anything).Return(nil).Times(2)

	s.env.ExecuteWorkflow(SpinDownWorkflow, SpinDownInput{
		Region:          "us-east-1",
		ClusterName:     "my-cluster",
		VpcID:           "vpc-123",
		ClusterRoleName: "my-cluster-eks-cluster-role",
		NodeRoleName:    "my-cluster-eks-node-role",
	})

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

// When role names are empty, SpinDownWorkflow must skip the IAM teardown child workflow.
func (s *InfraWorkflowTestSuite) Test_SpinDown_Success_WithoutRoles() {
	aws := &activities.AWSActivities{}
	s.env.OnActivity(aws.DeleteNodeGroup, mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity(aws.DeleteEKSCluster, mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity(aws.DeleteSubnets, mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity(aws.DeleteRouteTables, mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity(aws.DetachDeleteInternetGateway, mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity(aws.DeleteVPC, mock.Anything, mock.Anything).Return(nil)

	s.env.ExecuteWorkflow(SpinDownWorkflow, SpinDownInput{
		Region:      "us-east-1",
		ClusterName: "my-cluster",
		VpcID:       "vpc-123",
	})

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}
