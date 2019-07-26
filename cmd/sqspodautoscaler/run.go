package main

import (
	"k8s.io/klog"

	"github.com/spf13/cobra"

	"github.com/practo/k8s-sqs-pod-autoscaler-controller/pkg/cmdutil"
)

type runCmd struct {
	cmdutil.BaseCmd
}

var (
	runLong    = `Run the sqspodautoscaler`
	runExample = `  sqspodautoscaler run`
)

func (v *runCmd) new() *cobra.Command {
	v.Init("sqspodautoscaler", &cobra.Command{
		Use:     "run",
		Short:   "Run the sqspodautoscaler",
		Long:    runLong,
		Example: runExample,
		Run:     v.run,
	})

	return v.Cmd
}

func (v *runCmd) run(cmd *cobra.Command, args []string) {
	klog.Infof("Running the sqspodautoscaler")
}
