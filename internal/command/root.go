package command

import (
	"encoding/json"
	"fmt"
	"github.com/opencontainers/runtime-spec/specs-go"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

//execOptions
type execOptions struct {
	user       string
	config     string
	command    []string
	specConfig specConfig
}

type specConfig struct {
	Ropath       []string                `json:"readonlyPaths"`
	Capabilities specs.LinuxCapabilities `json:"capabilities"`
	UnmountPaths []string                `json:"unmountPaths"`
}

var rootCmd = newExecCommand()

func newExecOptions() execOptions {
	return execOptions{}
}

func newExecCommand() *cobra.Command {
	options := newExecOptions()

	cmd := &cobra.Command{
		Use:   "sandbox COMMAND [ARG...]",
		Short: "Run in a sandbox",
		Args:  RequiresMinArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options.command = args[0:]
			return runExec(options)
		},
	}

	flags := cmd.Flags()
	flags.SetInterspersed(false)
	flags.StringVarP(&options.user, "user", "u", "root", "User run in Sandbox")
	flags.StringVarP(&options.config, "config", "c", "./config", "Sandbox config path")
	return cmd
}

func buildCli(options execOptions) (*SandboxCli, error) {
	opts := []SandboxCliOption{}
	return NewSandboxCli(opts...)
}

func LoadConfig(options *execOptions) error {
	cf, err := os.Open(options.config)
	if err != nil {
		return err
	}
	defer cf.Close()
	err = json.NewDecoder(cf).Decode(&options.specConfig)
	options.specConfig.Ropath = RemoveDuplicateElement(options.specConfig.Ropath)
	if isDuplicate(options.specConfig.Ropath, options.specConfig.UnmountPaths) {
		return fmt.Errorf("there is duplication in readonlyPaths and unmountPaths")
	}
	return err
}

//Determine if there is duplication in readonlyPaths and unmountPaths
func isDuplicate(ro, unmount []string) bool {
	roMap := make(map[string]interface{})
	for _, path := range ro {
		roMap[path] = struct{}{}
	}
	for _, path := range unmount {
		if _, ok := roMap[path]; ok {
			return true
		}
	}

	return false
}

func RemoveDuplicateElement(languages []string) []string {
	result := make([]string, 0, len(languages))
	temp := map[string]struct{}{}
	for _, item := range languages {
		if _, ok := temp[item]; !ok {
			temp[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}

func runExec(options execOptions) error {
	logrus.SetLevel(logrus.ErrorLevel)
	cli, err := buildCli(options)
	if err != nil {
		return err
	}

	err = LoadConfig(&options)
	if err != nil {
		return err
	}
	spec, sandboxContainer, err := cli.CreateSandboxContainer(options)
	if err != nil {
		return err
	}

	err = cli.run(spec.Process, sandboxContainer)

	if err != nil {
		return err
	}
	return nil
}

// Execute main func
func Execute() {
	if os.Geteuid() != 0 {
		println("ERROR: please run sandbox with root")
		os.Exit(1)
	}
	if err := rootCmd.Execute(); err != nil {
		logrus.Debugf("%+v", err)

	}
}
