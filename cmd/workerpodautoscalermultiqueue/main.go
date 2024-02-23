package main

import (
	"flag"

	"github.com/practo/klog/v2"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/practo/k8s-worker-pod-autoscaler/pkg/cmdutil"
)

var rootCmd = &cobra.Command{
	Use:   "workerpodautoscaler",
	Short: "workerpodautoscaler scales the kubernetes deployments based on queue length",
	Long:  "workerpodautoscaler scales the kubernetes deployments based on queue length",
}

func init() {
	klog.InitFlags(nil)
	cobra.OnInitialize(func() {
		cmdutil.CheckErr(cmdutil.InitConfig("workerpodautoscalermultiqueue"))
	})

	rootCmd.PersistentFlags()
	pflag.CommandLine.AddGoFlag(flag.CommandLine.Lookup("v"))
}

func main() {
	versionCommand := (&versionCmd{}).new()
	runCommand := (&runCmd{}).new()

	// add main commands
	rootCmd.AddCommand(
		versionCommand,
		runCommand,
	)

	cmdutil.CheckErr(rootCmd.Execute())
}
