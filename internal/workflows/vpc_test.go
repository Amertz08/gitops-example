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

type VPCWorkflowTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
	env *testsuite.TestWorkflowEnvironment
}

func (s *VPCWorkflowTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *VPCWorkflowTestSuite) AfterTest(_, _ string) {
	s.env.AssertExpectations(s.T())
}

func TestVPCWorkflowSuite(t *testing.T) {
	suite.Run(t, new(VPCWorkflowTestSuite))
}

func (s *VPCWorkflowTestSuite) Test_SpinUpNetwork_InvalidInput() {
	s.env.ExecuteWorkflow(SpinUpNetworkWorkflow, SpinUpNetworkInput{})

	s.True(s.env.IsWorkflowCompleted())
	err := s.env.GetWorkflowError()
	s.Error(err)
	var appErr *temporal.ApplicationError
	s.True(errors.As(err, &appErr))
	s.Equal("InvalidInput", appErr.Type())
}

func (s *VPCWorkflowTestSuite) Test_SpinUpNetwork_Success() {
	aws := &activities.AWSActivities{}
	s.env.OnActivity(aws.CreateVPC, mock.Anything, mock.Anything).Return("vpc-123", nil)
	s.env.OnActivity(aws.CreateSubnets, mock.Anything, mock.Anything).
		Return([]string{"subnet-a", "subnet-b"}, nil)
	s.env.OnActivity(aws.CreateInternetGateway, mock.Anything, mock.Anything).Return("igw-123", nil)
	s.env.OnActivity(aws.ConfigureRouteTables, mock.Anything, mock.Anything).Return(nil)

	s.env.ExecuteWorkflow(SpinUpNetworkWorkflow, SpinUpNetworkInput{
		Region:      "us-east-1",
		Environment: "prod",
		Team:        "platform",
	})

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var out SpinUpNetworkOutput
	s.NoError(s.env.GetWorkflowResult(&out))
	s.Equal("vpc-123", out.VpcID)
	s.Equal([]string{"subnet-a", "subnet-b"}, out.SubnetIDs)
}

// CreateVPC fails before any compensation is registered, so no deletes should occur.
func (s *VPCWorkflowTestSuite) Test_SpinUpNetwork_CreateVPCFailure() {
	aws := &activities.AWSActivities{}
	s.env.OnActivity(aws.CreateVPC, mock.Anything, mock.Anything).
		Return("", errors.New("VPC limit reached"))

	s.env.ExecuteWorkflow(SpinUpNetworkWorkflow, SpinUpNetworkInput{
		Region:      "us-east-1",
		Environment: "prod",
		Team:        "platform",
	})

	s.True(s.env.IsWorkflowCompleted())
	s.Error(s.env.GetWorkflowError())
}

// CreateSubnets fails after VPC is created; saga must compensate by deleting the VPC.
func (s *VPCWorkflowTestSuite) Test_SpinUpNetwork_SubnetsFailure_CompensatesVPC() {
	aws := &activities.AWSActivities{}
	s.env.OnActivity(aws.CreateVPC, mock.Anything, mock.Anything).Return("vpc-123", nil)
	s.env.OnActivity(aws.CreateSubnets, mock.Anything, mock.Anything).
		Return([]string(nil), errors.New("subnet CIDR conflict"))
	s.env.OnActivity(aws.DeleteVPC, mock.Anything, mock.Anything).Return(nil).Once()

	s.env.ExecuteWorkflow(SpinUpNetworkWorkflow, SpinUpNetworkInput{
		Region:      "us-east-1",
		Environment: "prod",
		Team:        "platform",
	})

	s.True(s.env.IsWorkflowCompleted())
	s.Error(s.env.GetWorkflowError())
}

func (s *VPCWorkflowTestSuite) Test_SpinDownNetwork_InvalidInput() {
	s.env.ExecuteWorkflow(SpinDownNetworkWorkflow, SpinDownNetworkInput{})

	s.True(s.env.IsWorkflowCompleted())
	err := s.env.GetWorkflowError()
	s.Error(err)
	var appErr *temporal.ApplicationError
	s.True(errors.As(err, &appErr))
	s.Equal("InvalidInput", appErr.Type())
}

func (s *VPCWorkflowTestSuite) Test_SpinDownNetwork_Success() {
	aws := &activities.AWSActivities{}
	s.env.OnActivity(aws.DeleteSubnets, mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity(aws.DeleteRouteTables, mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity(aws.DetachDeleteInternetGateway, mock.Anything, mock.Anything).Return(nil)
	s.env.OnActivity(aws.DeleteVPC, mock.Anything, mock.Anything).Return(nil)

	s.env.ExecuteWorkflow(SpinDownNetworkWorkflow, SpinDownNetworkInput{
		Region: "us-east-1",
		VpcID:  "vpc-123",
	})

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())
}
