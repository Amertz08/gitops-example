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

var _ = Describe("SpinUpEKSWorkflow", func() {
	var (
		testSuite testsuite.WorkflowTestSuite
		env       *testsuite.TestWorkflowEnvironment
	)

	BeforeEach(func() {
		env = testSuite.NewTestWorkflowEnvironment()
	})

	AfterEach(func() {
		env.AssertExpectations(GinkgoT())
	})

	Context("with invalid input", func() {
		It("returns a non-retryable InvalidInput error", func() {
			env.ExecuteWorkflow(SpinUpEKSWorkflow, SpinUpEKSInput{})

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			err := env.GetWorkflowError()
			Expect(err).To(HaveOccurred())
			var appErr *temporal.ApplicationError
			Expect(errors.As(err, &appErr)).To(BeTrue())
			Expect(appErr.Type()).To(Equal("InvalidInput"))
		})
	})

	Context("with valid input", func() {
		It("creates the EKS cluster and node group", func() {
			aws := &activities.AWSActivities{}
			env.OnActivity(aws.CreateEKSCluster, mock.Anything, mock.Anything).Return(nil)
			env.OnActivity(aws.CreateNodeGroup, mock.Anything, mock.Anything).Return(nil)

			env.ExecuteWorkflow(SpinUpEKSWorkflow, validEKSInput())

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			Expect(env.GetWorkflowError()).NotTo(HaveOccurred())
		})
	})

	Context("when CreateNodeGroup fails", func() {
		It("compensates by deleting the EKS cluster", func() {
			aws := &activities.AWSActivities{}
			env.OnActivity(aws.CreateEKSCluster, mock.Anything, mock.Anything).Return(nil)
			env.OnActivity(aws.CreateNodeGroup, mock.Anything, mock.Anything).
				Return(errors.New("node group quota exceeded"))
			env.OnActivity(aws.DeleteEKSCluster, mock.Anything, mock.Anything).Return(nil).Once()

			env.ExecuteWorkflow(SpinUpEKSWorkflow, validEKSInput())

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			Expect(env.GetWorkflowError()).To(HaveOccurred())
		})
	})
})

var _ = Describe("SpinDownEKSWorkflow", func() {
	var (
		testSuite testsuite.WorkflowTestSuite
		env       *testsuite.TestWorkflowEnvironment
	)

	BeforeEach(func() {
		env = testSuite.NewTestWorkflowEnvironment()
	})

	AfterEach(func() {
		env.AssertExpectations(GinkgoT())
	})

	Context("with invalid input", func() {
		It("returns a non-retryable InvalidInput error", func() {
			env.ExecuteWorkflow(SpinDownEKSWorkflow, SpinDownEKSInput{})

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			err := env.GetWorkflowError()
			Expect(err).To(HaveOccurred())
			var appErr *temporal.ApplicationError
			Expect(errors.As(err, &appErr)).To(BeTrue())
			Expect(appErr.Type()).To(Equal("InvalidInput"))
		})
	})

	Context("with valid input", func() {
		It("deletes the node group then the EKS cluster", func() {
			aws := &activities.AWSActivities{}
			env.OnActivity(aws.DeleteNodeGroup, mock.Anything, mock.Anything).Return(nil)
			env.OnActivity(aws.DeleteEKSCluster, mock.Anything, mock.Anything).Return(nil)

			env.ExecuteWorkflow(SpinDownEKSWorkflow, SpinDownEKSInput{
				Region:      "us-east-1",
				ClusterName: "my-cluster",
			})

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			Expect(env.GetWorkflowError()).NotTo(HaveOccurred())
		})
	})
})
