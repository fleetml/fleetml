package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fleetml/fleetml/internal/version"
)

var (
	serverURL string
	outputFmt string
	verbose   bool
	cfg       *Config
	client    *Client
)

// NewRootCmd creates the root CLI command.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "fleetml",
		Short: "FleetML CLI — Edge ML model lifecycle management",
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			cfg = LoadConfig()
			if f := cmd.Flags().Lookup("server"); f != nil && f.Changed {
				cfg.Server = serverURL
			} else if serverURL == "" {
				serverURL = cfg.Server
			}
			if f := cmd.Flags().Lookup("output"); f != nil && f.Changed {
				cfg.Output = outputFmt
			} else if outputFmt == "" {
				outputFmt = cfg.Output
			}
			client = NewClient(serverURL)
			client.SetAPIKey(cfg.APIKey)
		},
		SilenceUsage: true,
	}

	root.PersistentFlags().StringVarP(&serverURL, "server", "s", "", "FleetML server URL (env: FLEETML_SERVER)")
	root.PersistentFlags().StringVarP(&outputFmt, "output", "o", "", "Output format: table or json")
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")

	root.AddCommand(newVersionCmd())
	root.AddCommand(newConfigCmd())
	root.AddCommand(newProjectCmd())
	root.AddCommand(newEnrollmentKeyCmd())
	root.AddCommand(newModelCmd())
	root.AddCommand(newVersionMgmtCmd())
	root.AddCommand(newPushCmd())
	root.AddCommand(newDeployCmd())
	root.AddCommand(newDeviceCmd())
	root.AddCommand(newTelemetryCmd())
	root.AddCommand(newPreviewCmd())
	root.AddCommand(newExperimentCmd())
	root.AddCommand(newDriftCmd())
	root.AddCommand(newSimulateCmd())

	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print FleetML CLI version",
		Run: func(_ *cobra.Command, _ []string) {
			PrintSuccess("fleetml " + versionString())
		},
	}
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage CLI configuration",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE: func(_ *cobra.Command, _ []string) error {
			PrintConfig(cfg)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "set-key [api-key]",
		Short: "Store the admin API key used for every request",
		Long: "Stores the API key in the config file. Without the argument the key is read from standard input:\n" +
			"typed at a hidden prompt in a terminal, or piped in (for example from a password manager),\n" +
			"which keeps it off the command line and out of the shell history.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := ""
			if len(args) == 1 {
				key = args[0]
			} else {
				var err error
				if key, err = readSecret(cmd.InOrStdin(), cmd.ErrOrStderr(), "API key (not shown): "); err != nil {
					return err
				}
			}
			key = strings.TrimSpace(key)
			if key == "" {
				return errors.New("no API key: type or paste it at the prompt, pipe it in, or pass it as the argument")
			}
			// Save what the file holds plus the key: not the environment's
			// values, and not the default server as if it had been set.
			fileCfg := loadSavedConfig()
			fileCfg.APIKey = key
			if err := SaveConfig(fileCfg); err != nil {
				return err
			}
			PrintSuccess("api key saved to " + configPath())
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "set [key] [value]",
		Short: "Set a configuration value",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			fileCfg := loadSavedConfig()
			switch args[0] {
			case "server":
				fileCfg.Server = args[1]
			case "output":
				fileCfg.Output = args[1]
			case "project":
				fileCfg.Project = args[1]
			default:
				return ErrUnknownConfigKey(args[0])
			}
			if err := SaveConfig(fileCfg); err != nil {
				return err
			}
			PrintSuccess("config updated")
			return nil
		},
	})
	return cmd
}

// versionString is "v0.5.0" for a release build and "dev" otherwise.
func versionString() string {
	if v := version.String(); v != "dev" {
		return "v" + v
	}
	return "dev"
}

// ErrUnknownConfigKey returns an error for unknown config keys.
func ErrUnknownConfigKey(key string) error {
	return &configError{key: key}
}

type configError struct {
	key string
}

func (e *configError) Error() string {
	return "unknown config key: " + e.key
}
