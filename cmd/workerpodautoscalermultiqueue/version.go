package main

import (
	"fmt"
	"github.com/spf13/cobra"

	"github.com/practo/k8s-worker-pod-autoscaler/pkg/cmdutil"
	"github.com/practo/k8s-worker-pod-autoscaler/pkg/version"
)

type versionCmd struct {
	cmdutil.BaseCmd
}

var (
	versionLong    = `Display the version`
	versionExample = `  workerpodautoscaler version`
)

func (v *versionCmd) new() *cobra.Command {
	v.Init("workerpodautoscaler", &cobra.Command{
		Use:     "version",
		Short:   "Display the version",
		Long:    versionLong,
		Aliases: []string{"v", "ver"},
		Example: versionExample,
		Run:     v.run,
	})

	return v.Cmd
}

func (v *versionCmd) run(cmd *cobra.Command, args []string) {
	fmt.Println("Version " + version.GetVersion())
}
