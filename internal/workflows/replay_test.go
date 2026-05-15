package workflows

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.temporal.io/sdk/worker"
)

var _ = Describe("Replay", Label("replay"), func() {
	BeforeEach(func() {
		if os.Getenv("RUN_REPLAY_TESTS") == "" {
			Skip("set RUN_REPLAY_TESTS=1 to run replay tests")
		}
	})

	It("SpinUpWorkflow is deterministic against recorded history", func() {
		r := worker.NewWorkflowReplayer()
		r.RegisterWorkflow(SpinUpWorkflow)
		Expect(r.ReplayWorkflowHistoryFromJSONFile(nil, "testdata/spin_up_history.json")).
			NotTo(HaveOccurred())
	})

	It("SpinDownWorkflow is deterministic against recorded history", func() {
		r := worker.NewWorkflowReplayer()
		r.RegisterWorkflow(SpinDownWorkflow)
		Expect(r.ReplayWorkflowHistoryFromJSONFile(nil, "testdata/spin_down_history.json")).
			NotTo(HaveOccurred())
	})

	It("SpinUpIAMWorkflow is deterministic against recorded history", func() {
		r := worker.NewWorkflowReplayer()
		r.RegisterWorkflow(SpinUpIAMWorkflow)
		Expect(r.ReplayWorkflowHistoryFromJSONFile(nil, "testdata/spin_up_iam_history.json")).
			NotTo(HaveOccurred())
	})

	It("SpinDownIAMWorkflow is deterministic against recorded history", func() {
		r := worker.NewWorkflowReplayer()
		r.RegisterWorkflow(SpinDownIAMWorkflow)
		Expect(r.ReplayWorkflowHistoryFromJSONFile(nil, "testdata/spin_down_iam_history.json")).
			NotTo(HaveOccurred())
	})

	It("SpinUpNetworkWorkflow is deterministic against recorded history", func() {
		r := worker.NewWorkflowReplayer()
		r.RegisterWorkflow(SpinUpNetworkWorkflow)
		Expect(r.ReplayWorkflowHistoryFromJSONFile(nil, "testdata/spin_up_network_history.json")).
			NotTo(HaveOccurred())
	})

	It("SpinDownNetworkWorkflow is deterministic against recorded history", func() {
		r := worker.NewWorkflowReplayer()
		r.RegisterWorkflow(SpinDownNetworkWorkflow)
		Expect(r.ReplayWorkflowHistoryFromJSONFile(nil, "testdata/spin_down_network_history.json")).
			NotTo(HaveOccurred())
	})

	It("SpinUpEKSWorkflow is deterministic against recorded history", func() {
		r := worker.NewWorkflowReplayer()
		r.RegisterWorkflow(SpinUpEKSWorkflow)
		Expect(r.ReplayWorkflowHistoryFromJSONFile(nil, "testdata/spin_up_eks_history.json")).
			NotTo(HaveOccurred())
	})

	It("SpinDownEKSWorkflow is deterministic against recorded history", func() {
		r := worker.NewWorkflowReplayer()
		r.RegisterWorkflow(SpinDownEKSWorkflow)
		Expect(r.ReplayWorkflowHistoryFromJSONFile(nil, "testdata/spin_down_eks_history.json")).
			NotTo(HaveOccurred())
	})
})
