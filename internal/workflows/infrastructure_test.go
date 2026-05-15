package workflows

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/Amertz08/gitops-example/internal/activities"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
)

func mockHappyPathNetworkActivities(env *testsuite.TestWorkflowEnvironment) {
	aws := &activities.AWSActivities{}
	env.OnActivity(aws.CreateVPC, mock.Anything, mock.Anything).Return("vpc-123", nil)
	env.OnActivity(aws.CreateSubnets, mock.Anything, mock.Anything).
		Return([]string{"subnet-a", "subnet-b"}, nil)
	env.OnActivity(aws.CreateInternetGateway, mock.Anything, mock.Anything).Return("igw-123", nil)
	env.OnActivity(aws.ConfigureRouteTables, mock.Anything, mock.Anything).Return(nil)
}

func mockHappyPathEKSActivities(env *testsuite.TestWorkflowEnvironment) {
	aws := &activities.AWSActivities{}
	env.OnActivity(aws.CreateEKSCluster, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity(aws.CreateNodeGroup, mock.Anything, mock.Anything).Return(nil)
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

var _ = Describe("SpinUpWorkflow", func() {
	var (
		testSuite testsuite.WorkflowTestSuite
		env       *testsuite.TestWorkflowEnvironment
	)

	BeforeEach(func() {
		env = testSuite.NewTestWorkflowEnvironment()
		env.RegisterWorkflow(SpinUpNetworkWorkflow)
		env.RegisterWorkflow(SpinUpIAMWorkflow)
		env.RegisterWorkflow(SpinUpEKSWorkflow)
	})

	AfterEach(func() {
		env.AssertExpectations(GinkgoT())
	})

	Context("with invalid input", func() {
		It("returns a non-retryable InvalidInput error", func() {
			env.ExecuteWorkflow(SpinUpWorkflow, SpinUpInput{})

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			err := env.GetWorkflowError()
			Expect(err).To(HaveOccurred())
			var appErr *temporal.ApplicationError
			Expect(errors.As(err, &appErr)).To(BeTrue())
			Expect(appErr.Type()).To(Equal("InvalidInput"))
		})
	})

	Context("without pre-supplied IAM role ARNs", func() {
		It("runs SpinUpIAMWorkflow concurrently with SpinUpNetworkWorkflow then SpinUpEKSWorkflow",
			func() {
				aws := &activities.AWSActivities{}
				env.OnActivity(aws.CreateIAMRole, mock.Anything, mock.Anything).
					Return("arn:aws:iam::123:role/cluster-role", nil).Once()
				env.OnActivity(aws.CreateIAMRole, mock.Anything, mock.Anything).
					Return("arn:aws:iam::123:role/node-role", nil).Once()
				mockHappyPathNetworkActivities(env)
				mockHappyPathEKSActivities(env)

				env.ExecuteWorkflow(SpinUpWorkflow, validSpinUpInput())

				Expect(env.IsWorkflowCompleted()).To(BeTrue())
				Expect(env.GetWorkflowError()).NotTo(HaveOccurred())
			})
	})

	Context("with pre-supplied IAM role ARNs", func() {
		It("skips SpinUpIAMWorkflow and uses the provided ARNs directly", func() {
			mockHappyPathNetworkActivities(env)
			mockHappyPathEKSActivities(env)

			input := validSpinUpInput()
			input.ClusterRoleARN = "arn:aws:iam::123:role/existing-cluster-role"
			input.NodeRoleARN = "arn:aws:iam::123:role/existing-node-role"
			env.ExecuteWorkflow(SpinUpWorkflow, input)

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			Expect(env.GetWorkflowError()).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("SpinDownWorkflow", func() {
	var (
		testSuite testsuite.WorkflowTestSuite
		env       *testsuite.TestWorkflowEnvironment
	)

	BeforeEach(func() {
		env = testSuite.NewTestWorkflowEnvironment()
		env.RegisterWorkflow(SpinDownEKSWorkflow)
		env.RegisterWorkflow(SpinDownNetworkWorkflow)
		env.RegisterWorkflow(SpinDownIAMWorkflow)
	})

	AfterEach(func() {
		env.AssertExpectations(GinkgoT())
	})

	Context("with invalid input", func() {
		It("returns a non-retryable InvalidInput error", func() {
			env.ExecuteWorkflow(SpinDownWorkflow, SpinDownInput{})

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			err := env.GetWorkflowError()
			Expect(err).To(HaveOccurred())
			var appErr *temporal.ApplicationError
			Expect(errors.As(err, &appErr)).To(BeTrue())
			Expect(appErr.Type()).To(Equal("InvalidInput"))
		})
	})

	Context("with role names provided", func() {
		It("tears down EKS, network, and IAM roles", func() {
			aws := &activities.AWSActivities{}
			env.OnActivity(aws.DeleteNodeGroup, mock.Anything, mock.Anything).Return(nil)
			env.OnActivity(aws.DeleteEKSCluster, mock.Anything, mock.Anything).Return(nil)
			env.OnActivity(aws.DeleteSubnets, mock.Anything, mock.Anything).Return(nil)
			env.OnActivity(aws.DeleteRouteTables, mock.Anything, mock.Anything).Return(nil)
			env.OnActivity(aws.DetachDeleteInternetGateway, mock.Anything, mock.Anything).
				Return(nil)
			env.OnActivity(aws.DeleteVPC, mock.Anything, mock.Anything).Return(nil)
			env.OnActivity(aws.DeleteIAMRole, mock.Anything, mock.Anything).Return(nil).Times(2)

			env.ExecuteWorkflow(SpinDownWorkflow, SpinDownInput{
				Region:          "us-east-1",
				ClusterName:     "my-cluster",
				VpcID:           "vpc-123",
				ClusterRoleName: "my-cluster-eks-cluster-role",
				NodeRoleName:    "my-cluster-eks-node-role",
			})

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			Expect(env.GetWorkflowError()).NotTo(HaveOccurred())
		})
	})

	Context("without role names", func() {
		It("tears down EKS and network, skipping the IAM child workflow", func() {
			aws := &activities.AWSActivities{}
			env.OnActivity(aws.DeleteNodeGroup, mock.Anything, mock.Anything).Return(nil)
			env.OnActivity(aws.DeleteEKSCluster, mock.Anything, mock.Anything).Return(nil)
			env.OnActivity(aws.DeleteSubnets, mock.Anything, mock.Anything).Return(nil)
			env.OnActivity(aws.DeleteRouteTables, mock.Anything, mock.Anything).Return(nil)
			env.OnActivity(aws.DetachDeleteInternetGateway, mock.Anything, mock.Anything).
				Return(nil)
			env.OnActivity(aws.DeleteVPC, mock.Anything, mock.Anything).Return(nil)

			env.ExecuteWorkflow(SpinDownWorkflow, SpinDownInput{
				Region:      "us-east-1",
				ClusterName: "my-cluster",
				VpcID:       "vpc-123",
			})

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			Expect(env.GetWorkflowError()).NotTo(HaveOccurred())
		})
	})
})
