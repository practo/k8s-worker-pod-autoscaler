package cmdutil

import (
	"fmt"
	"os"
	"path/filepath"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
	"k8s.io/klog"
)

// InitConfig constructs the configuration from a local configuration file
// or environment variables if available. This is placed in the global `viper`
// instance.
func InitConfig(name string) error {
	home, err := homedir.Dir()
	if err != nil {
		return err
	}

	cfgDir := filepath.Join(home, ".sqspodautoscaler")
	if _, statErr := os.Stat(cfgDir); os.IsNotExist(statErr) {
		// #nosec
		if mkdirErr := os.Mkdir(cfgDir, 0755); mkdirErr != nil {
			return mkdirErr
		}
	}
	cfgName := fmt.Sprintf("%s.yaml", name)
	cfgPath := filepath.Join(cfgDir, cfgName)

	fobj, err := os.OpenFile(cfgPath, os.O_CREATE|os.O_WRONLY, 0644) // #nosec
	if err != nil {
		return err
	}
	defer fobj.Close() // nolint: errcheck

	viper.SetConfigFile(cfgPath)
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return err
	}

	klog.V(99).Infof("Using config file: %s", viper.ConfigFileUsed())

	return nil
}

// ConfigPath returns the directory path being used by config.
func ConfigPath() string {
	return filepath.Dir(viper.ConfigFileUsed())
}
