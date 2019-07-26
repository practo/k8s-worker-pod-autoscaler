package main

import (
	"k8s.io/klog"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/practo/k8s-sqs-pod-autoscaler-controller/pkg/cmdutil"
)

var rootCmd = &cobra.Command{
	Use:   "sqspodautoscaler",
	Short: "sqspodautoscaler scales the kubernetes deployments based on aws sqs queue length",
	Long:  "sqspodautoscaler scales the kubernetes deployments based on aws sqs queue length",
}

func localFlags(flags *pflag.FlagSet) {
}

func init() {
	cobra.OnInitialize(func() {
		cmdutil.CheckErr(cmdutil.InitConfig("sqspodautoscaler"))
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
