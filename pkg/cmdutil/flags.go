package cmdutil

import (
	"fmt"
	"strings"

	// "github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// BindFlag moves cobra flags into viper for exclusive use there.
func BindFlag(v *viper.Viper, flag *pflag.Flag, root string) error {
	if err := v.BindPFlag(flag.Name, flag); err != nil {
		return err
	}

	if err := v.BindEnv(
		flag.Name,
		strings.Replace(
			strings.ToUpper(
				fmt.Sprintf("%s_%s", root, flag.Name)), "-", "_", -1)); err != nil {

		return err
	}

	return nil
}
