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

var _ = Describe("SpinUpNetworkWorkflow", func() {
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
			env.ExecuteWorkflow(SpinUpNetworkWorkflow, SpinUpNetworkInput{})

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			err := env.GetWorkflowError()
			Expect(err).To(HaveOccurred())
			var appErr *temporal.ApplicationError
			Expect(errors.As(err, &appErr)).To(BeTrue())
			Expect(appErr.Type()).To(Equal("InvalidInput"))
		})
	})

	Context("with valid input", func() {
		It("creates VPC, subnets, IGW, and route tables, returning VPC ID and subnet IDs", func() {
			aws := &activities.AWSActivities{}
			env.OnActivity(aws.CreateVPC, mock.Anything, mock.Anything).Return("vpc-123", nil)
			env.OnActivity(aws.CreateSubnets, mock.Anything, mock.Anything).
				Return([]string{"subnet-a", "subnet-b"}, nil)
			env.OnActivity(aws.CreateInternetGateway, mock.Anything, mock.Anything).
				Return("igw-123", nil)
			env.OnActivity(aws.ConfigureRouteTables, mock.Anything, mock.Anything).Return(nil)

			env.ExecuteWorkflow(SpinUpNetworkWorkflow, SpinUpNetworkInput{
				Region:      "us-east-1",
				Environment: "prod",
				Team:        "platform",
			})

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			Expect(env.GetWorkflowError()).NotTo(HaveOccurred())

			var out SpinUpNetworkOutput
			Expect(env.GetWorkflowResult(&out)).NotTo(HaveOccurred())
			Expect(out.VpcID).To(Equal("vpc-123"))
			Expect(out.SubnetIDs).To(Equal([]string{"subnet-a", "subnet-b"}))
		})
	})

	Context("when CreateVPC fails", func() {
		It("returns an error without triggering any compensation", func() {
			aws := &activities.AWSActivities{}
			env.OnActivity(aws.CreateVPC, mock.Anything, mock.Anything).
				Return("", errors.New("VPC limit reached"))

			env.ExecuteWorkflow(SpinUpNetworkWorkflow, SpinUpNetworkInput{
				Region:      "us-east-1",
				Environment: "prod",
				Team:        "platform",
			})

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			Expect(env.GetWorkflowError()).To(HaveOccurred())
		})
	})

	Context("when CreateSubnets fails after VPC is created", func() {
		It("compensates by deleting the VPC", func() {
			aws := &activities.AWSActivities{}
			env.OnActivity(aws.CreateVPC, mock.Anything, mock.Anything).Return("vpc-123", nil)
			env.OnActivity(aws.CreateSubnets, mock.Anything, mock.Anything).
				Return([]string(nil), errors.New("subnet CIDR conflict"))
			env.OnActivity(aws.DeleteVPC, mock.Anything, mock.Anything).Return(nil).Once()

			env.ExecuteWorkflow(SpinUpNetworkWorkflow, SpinUpNetworkInput{
				Region:      "us-east-1",
				Environment: "prod",
				Team:        "platform",
			})

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			Expect(env.GetWorkflowError()).To(HaveOccurred())
		})
	})
})

var _ = Describe("SpinDownNetworkWorkflow", func() {
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
			env.ExecuteWorkflow(SpinDownNetworkWorkflow, SpinDownNetworkInput{})

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			err := env.GetWorkflowError()
			Expect(err).To(HaveOccurred())
			var appErr *temporal.ApplicationError
			Expect(errors.As(err, &appErr)).To(BeTrue())
			Expect(appErr.Type()).To(Equal("InvalidInput"))
		})
	})

	Context("with valid input", func() {
		It("deletes subnets, route tables, IGW, and VPC in order", func() {
			aws := &activities.AWSActivities{}
			env.OnActivity(aws.DeleteSubnets, mock.Anything, mock.Anything).Return(nil)
			env.OnActivity(aws.DeleteRouteTables, mock.Anything, mock.Anything).Return(nil)
			env.OnActivity(aws.DetachDeleteInternetGateway, mock.Anything, mock.Anything).
				Return(nil)
			env.OnActivity(aws.DeleteVPC, mock.Anything, mock.Anything).Return(nil)

			env.ExecuteWorkflow(SpinDownNetworkWorkflow, SpinDownNetworkInput{
				Region: "us-east-1",
				VpcID:  "vpc-123",
			})

			Expect(env.IsWorkflowCompleted()).To(BeTrue())
			Expect(env.GetWorkflowError()).NotTo(HaveOccurred())
		})
	})
})
