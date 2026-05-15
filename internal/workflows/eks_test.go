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

type EKSWorkflowTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
	env *testsuite.TestWorkflowEnvironment
}

func (s *EKSWorkflowTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *EKSWorkflowTestSuite) AfterTest(_, _ string) {
	s.env.AssertExpectations(s.T())
}

func TestEKSWorkflowSuite(t *testing.T) {
	suite.Run(t, new(EKSWorkflowTestSuite))
}

func validEKSInput() SpinUpEKSInput {
	return SpinUpEKSInput{
		Region:           "us-east-1",
		ClusterName:      "my-cluster",
		ClusterRoleARN:   "arn:aws:iam::123:role/cluster-role",
		NodeRoleARN:      "arn:aws:iam::123:role/node-role",
		VpcID:            "vpc-123",
		SubnetIDs:        []string{"subnet-a", "subnet-b"},
		NodeCount:        2,
		NodeInstanceType: "t3.medium",
		Environment:      "prod",
		Team:             "platform",
	}
}

func (s *EKSWorkflowTestSuite) Test_SpinUpEKS_InvalidInput() {
	s.env.ExecuteWorkflow(SpinUpEKSWorkflow, SpinUpEKSInput{})

	s.True(s.env.IsWorkflowCompleted())
	err := s.env.GetWorkflowError()
	s.Error(err)
	var appErr *temporal.ApplicationError
	s.True(errors.As(err, &appErr))
	s.Equal("InvalidInput", appErr.Type())
}

func (s *EKSWorkflowTestSuite) Test_SpinUpEKS_Success() {
	aws := &activities.AWSActivities{}
	s.env.OnActivity(aws.CreateEKSCluster, mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity(aws.CreateNodeGroup, mock.Anything, mock.Anything).Return(nil)

	s.env.ExecuteWorkflow(SpinUpEKSWorkflow, validEKSInput())

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}

// CreateNodeGroup failing must trigger saga compensation to delete the cluster.
func (s *EKSWorkflowTestSuite) Test_SpinUpEKS_NodeGroupFailure_CompensatesCluster() {
	aws := &activities.AWSActivities{}
	s.env.OnActivity(aws.CreateEKSCluster, mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity(aws.CreateNodeGroup, mock.Anything, mock.Anything).
		Return(errors.New("node group quota exceeded"))
	s.env.OnActivity(aws.DeleteEKSCluster, mock.Anything, mock.Anything).Return(nil).Once()

	s.env.ExecuteWorkflow(SpinUpEKSWorkflow, validEKSInput())

	s.True(s.env.IsWorkflowCompleted())
	s.Error(s.env.GetWorkflowError())
}

func (s *EKSWorkflowTestSuite) Test_SpinDownEKS_InvalidInput() {
	s.env.ExecuteWorkflow(SpinDownEKSWorkflow, SpinDownEKSInput{})

	s.True(s.env.IsWorkflowCompleted())
	err := s.env.GetWorkflowError()
	s.Error(err)
	var appErr *temporal.ApplicationError
	s.True(errors.As(err, &appErr))
	s.Equal("InvalidInput", appErr.Type())
}

func (s *EKSWorkflowTestSuite) Test_SpinDownEKS_Success() {
	aws := &activities.AWSActivities{}
	s.env.OnActivity(aws.DeleteNodeGroup, mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity(aws.DeleteEKSCluster, mock.Anything, mock.Anything).Return(nil)

	s.env.ExecuteWorkflow(SpinDownEKSWorkflow, SpinDownEKSInput{
		Region:      "us-east-1",
		ClusterName: "my-cluster",
	})

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}
