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

var _ = Describe("SpinUpIAMWorkflow", func() {
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
			env.ExecuteWorkflow(SpinUpIAMWorkflow, SpinUpEKSIAMInput{})

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			err := env.GetWorkflowError()
			Expect(err).To(HaveOccurred())
			var appErr *temporal.ApplicationError
			Expect(errors.As(err, &appErr)).To(BeTrue())
			Expect(appErr.Type()).To(Equal("InvalidInput"))
		})
	})

	Context("with valid input", func() {
		It("creates both IAM roles and returns their ARNs and names", func() {
			aws := &activities.AWSActivities{}
			env.OnActivity(aws.CreateIAMRole, mock.Anything, mock.Anything).
				Return("arn:aws:iam::123:role/cluster-role", nil).Once()
			env.OnActivity(aws.CreateIAMRole, mock.Anything, mock.Anything).
				Return("arn:aws:iam::123:role/node-role", nil).Once()

			env.ExecuteWorkflow(SpinUpIAMWorkflow, SpinUpEKSIAMInput{
				ClusterName: "my-cluster",
				Environment: "prod",
				Team:        "platform",
			})

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			Expect(env.GetWorkflowError()).NotTo(HaveOccurred())

			var out SpinUpEKSIAMOutput
			Expect(env.GetWorkflowResult(&out)).NotTo(HaveOccurred())
			Expect(out.ClusterRoleARN).To(Equal("arn:aws:iam::123:role/cluster-role"))
			Expect(out.ClusterRoleName).To(Equal("my-cluster-eks-cluster-role"))
			Expect(out.NodeRoleARN).To(Equal("arn:aws:iam::123:role/node-role"))
			Expect(out.NodeRoleName).To(Equal("my-cluster-eks-node-role"))
		})
	})

	Context("when node role creation fails", func() {
		It("compensates by deleting the cluster role", func() {
			aws := &activities.AWSActivities{}
			env.OnActivity(aws.CreateIAMRole, mock.Anything, mock.Anything).
				Return("arn:aws:iam::123:role/cluster-role", nil).Once()
			env.OnActivity(aws.CreateIAMRole, mock.Anything, mock.Anything).
				Return("", errors.New("IAM quota exceeded"))
			env.OnActivity(aws.DeleteIAMRole, mock.Anything, mock.Anything).
				Return(nil).Once()

			env.ExecuteWorkflow(SpinUpIAMWorkflow, SpinUpEKSIAMInput{
				ClusterName: "my-cluster",
				Environment: "prod",
				Team:        "platform",
			})

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			Expect(env.GetWorkflowError()).To(HaveOccurred())
		})
	})
})

var _ = Describe("SpinDownIAMWorkflow", func() {
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
			env.ExecuteWorkflow(SpinDownIAMWorkflow, SpinDownEKSIAMInput{})

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			err := env.GetWorkflowError()
			Expect(err).To(HaveOccurred())
			var appErr *temporal.ApplicationError
			Expect(errors.As(err, &appErr)).To(BeTrue())
			Expect(appErr.Type()).To(Equal("InvalidInput"))
		})
	})

	Context("with valid input", func() {
		It("deletes both IAM roles", func() {
			aws := &activities.AWSActivities{}
			env.OnActivity(aws.DeleteIAMRole, mock.Anything, mock.Anything).Return(nil).Times(2)

			env.ExecuteWorkflow(SpinDownIAMWorkflow, SpinDownEKSIAMInput{
				ClusterRoleName: "my-cluster-eks-cluster-role",
				NodeRoleName:    "my-cluster-eks-node-role",
			})

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			Expect(env.GetWorkflowError()).NotTo(HaveOccurred())
		})
	})
})
