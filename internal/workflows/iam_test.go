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

type IAMWorkflowTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
	env *testsuite.TestWorkflowEnvironment
}

func (s *IAMWorkflowTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *IAMWorkflowTestSuite) AfterTest(_, _ string) {
	s.env.AssertExpectations(s.T())
}

func TestIAMWorkflowSuite(t *testing.T) {
	suite.Run(t, new(IAMWorkflowTestSuite))
}

func (s *IAMWorkflowTestSuite) Test_SpinUpIAM_InvalidInput() {
	s.env.ExecuteWorkflow(SpinUpIAMWorkflow, SpinUpEKSIAMInput{})

	s.True(s.env.IsWorkflowCompleted())
	err := s.env.GetWorkflowError()
	s.Error(err)
	var appErr *temporal.ApplicationError
	s.True(errors.As(err, &appErr))
	s.Equal("InvalidInput", appErr.Type())
}

func (s *IAMWorkflowTestSuite) Test_SpinUpIAM_Success() {
	aws := &activities.AWSActivities{}
	s.env.OnActivity(aws.CreateIAMRole, mock.Anything, mock.Anything).
		Return("arn:aws:iam::123:role/cluster-role", nil).Once()
	s.env.OnActivity(aws.CreateIAMRole, mock.Anything, mock.Anything).
		Return("arn:aws:iam::123:role/node-role", nil).Once()

	s.env.ExecuteWorkflow(SpinUpIAMWorkflow, SpinUpEKSIAMInput{
		ClusterName: "my-cluster",
		Environment: "prod",
		Team:        "platform",
	})

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var out SpinUpEKSIAMOutput
	s.NoError(s.env.GetWorkflowResult(&out))
	s.Equal("arn:aws:iam::123:role/cluster-role", out.ClusterRoleARN)
	s.Equal("my-cluster-eks-cluster-role", out.ClusterRoleName)
	s.Equal("arn:aws:iam::123:role/node-role", out.NodeRoleARN)
	s.Equal("my-cluster-eks-node-role", out.NodeRoleName)
}

// When node role creation fails, the saga must compensate the already-created cluster role.
func (s *IAMWorkflowTestSuite) Test_SpinUpIAM_NodeRoleFailure_CompensatesClusterRole() {
	aws := &activities.AWSActivities{}
	s.env.OnActivity(aws.CreateIAMRole, mock.Anything, mock.Anything).
		Return("arn:aws:iam::123:role/cluster-role", nil).Once()
	s.env.OnActivity(aws.CreateIAMRole, mock.Anything, mock.Anything).
		Return("", errors.New("IAM quota exceeded"))
	s.env.OnActivity(aws.DeleteIAMRole, mock.Anything, mock.Anything).
		Return(nil).Once()

	s.env.ExecuteWorkflow(SpinUpIAMWorkflow, SpinUpEKSIAMInput{
		ClusterName: "my-cluster",
		Environment: "prod",
		Team:        "platform",
	})

	s.True(s.env.IsWorkflowCompleted())
	s.Error(s.env.GetWorkflowError())
}

func (s *IAMWorkflowTestSuite) Test_SpinDownIAM_InvalidInput() {
	s.env.ExecuteWorkflow(SpinDownIAMWorkflow, SpinDownEKSIAMInput{})

	s.True(s.env.IsWorkflowCompleted())
	err := s.env.GetWorkflowError()
	s.Error(err)
	var appErr *temporal.ApplicationError
	s.True(errors.As(err, &appErr))
	s.Equal("InvalidInput", appErr.Type())
}

func (s *IAMWorkflowTestSuite) Test_SpinDownIAM_Success() {
	aws := &activities.AWSActivities{}
	s.env.OnActivity(aws.DeleteIAMRole, mock.Anything, mock.Anything).Return(nil).Times(2)

	s.env.ExecuteWorkflow(SpinDownIAMWorkflow, SpinDownEKSIAMInput{
		ClusterRoleName: "my-cluster-eks-cluster-role",
		NodeRoleName:    "my-cluster-eks-node-role",
	})

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}
