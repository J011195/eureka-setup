/*
Copyright © 2024 EPAM_Systems/Thunderjet/Boburbek_Kadirkhodjaev

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"

	"github.com/folio-org/eureka-cli/internal"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const rootCommand string = "Root"

var (
	withConfigFile        string
	withProfile           string
	withModuleName        string
	withEnableDebug       bool
	withBuildImages       bool
	withUpdateCloned      bool
	withEnableEcsRequests bool
	withTenant            string
	withNamespace         string
	withShowAll           bool
	withId                string
	withModuleUrl         string
	withSidecarUrl        string
	withRestore           bool
	withDefaultGateway    bool
	withRequired          bool
	withUser              string
	withLength            int
)

var embeddedFs embed.FS

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "eureka-cli",
	Short: "Eureka CLI",
	Long:  `Eureka CLI to deploy a local development environment.`,
}

func Execute(mainEmbeddedFs embed.FS) {
	embeddedFs = mainEmbeddedFs
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func initConfig() {
	setConfig(withEnableDebug, withConfigFile, withProfile)

	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		recursivelySetupHomeConfigDir(withEnableDebug, embeddedFs)
	}
}

func setConfig(enabledDebug bool, configFile string, profile string) {
	if configFile == "" {
		setConfigNameByProfile(enabledDebug, profile)
		return
	}

	viper.SetConfigFile(configFile)
}

func setConfigNameByProfile(enabledDebug bool, profile string) {
	if enabledDebug {
		slog.Info(rootCommand, internal.GetFuncName(), fmt.Sprintf("Using profile: %s", profile))
	}

	home, err := os.UserHomeDir()
	cobra.CheckErr(err)

	configPath := path.Join(home, internal.ConfigDir)
	viper.AddConfigPath(configPath)
	viper.SetConfigType(internal.ConfigType)

	configFile := getConfigFileByProfile(profile)
	viper.SetConfigName(configFile)
}

func getConfigFileByProfile(profile string) string {
	if profile == "" {
		profile = internal.AvailableConfigs[0]
	}

	return fmt.Sprintf("config.%s", profile)
}

func recursivelySetupHomeConfigDir(enabledDebug bool, embeddedFs embed.FS) {
	homeConfigDir := internal.GetHomeConfigDir(rootCommand)

	err := fs.WalkDir(embeddedFs, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		dstPath := filepath.Join(homeConfigDir, path)
		if d.IsDir() {
			err := os.MkdirAll(dstPath, 0755)
			if err != nil {
				return err
			}

			if enabledDebug {
				slog.Info(rootCommand, internal.GetFuncName(), fmt.Sprintf("Created directory: %s", homeConfigDir))
			}
		} else {
			content, err := fs.ReadFile(embeddedFs, path)
			if err != nil {
				return err
			}

			err = os.WriteFile(dstPath, content, 0644)
			if err != nil {
				return err
			}

			if enabledDebug {
				slog.Info(rootCommand, internal.GetFuncName(), fmt.Sprintf("Created file: %s", dstPath))
			}
		}

		return nil
	})
	cobra.CheckErr(err)
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVarP(&withProfile, "profile", "p", "", fmt.Sprintf("Profile, available profile: %s, default profile: %s", internal.AvailableConfigs, internal.AvailableConfigs[0]))
	rootCmd.PersistentFlags().StringVarP(&withConfigFile, "config", "c", "", fmt.Sprintf("Config file, default config file: config.%s.%s", internal.AvailableConfigs[0], internal.ConfigType))
	rootCmd.PersistentFlags().BoolVarP(&withEnableDebug, "debug", "d", false, "Enable HTTP and other debug output")
}
