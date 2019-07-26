package main

import (
	"k8s.io/klog"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/practo/k8s-worker-pod-autoscaler/pkg/cmdutil"
)

var rootCmd = &cobra.Command{
	Use:   "workerpodautoscaler",
	Short: "workerpodautoscaler scales the kubernetes deployments based on queue length",
	Long:  "workerpodautoscaler scales the kubernetes deployments based on queue length",
}

func localFlags(flags *pflag.FlagSet) {
}

func init() {
	cobra.OnInitialize(func() {
		cmdutil.CheckErr(cmdutil.InitConfig("workerpodautoscaler"))
	})

	flags := rootCmd.PersistentFlags()
	localFlags(flags)
}

func main() {
	klog.InitFlags(nil)

	versionCommand := (&versionCmd{}).new()
	runCommand := (&runCmd{}).new()

	// add main commands
	rootCmd.AddCommand(
		versionCommand,
		runCommand,
	)

	cmdutil.CheckErr(rootCmd.Execute())
}
