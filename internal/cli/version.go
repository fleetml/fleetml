package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionMgmtCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "ver", Short: "Manage model versions"}
	cmd.AddCommand(newVerListCmd())
	cmd.AddCommand(newVerCreateCmd())
	cmd.AddCommand(newVerPromoteCmd())
	return cmd
}

func newVerListCmd() *cobra.Command {
	var modelID string
	cmd := &cobra.Command{
		Use: "list", Short: "List versions",
		RunE: func(_ *cobra.Command, _ []string) error {
			var result []map[string]interface{}
			if err := client.Get("/api/v1/models/"+modelID+"/versions", &result); err != nil {
				return err
			}
			if outputFmt == "json" {
				PrintJSON(result)
				return nil
			}
			var rows [][]string
			for _, v := range result {
				rows = append(rows, []string{fmt.Sprint(v["ID"]), fmt.Sprint(v["Version"]), fmt.Sprint(v["Status"])})
			}
			PrintTable([]string{"ID", "Version", "Status"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&modelID, "model", "", "Model ID (required)")
	_ = cmd.MarkFlagRequired("model")
	return cmd
}

func newVerCreateCmd() *cobra.Command {
	var modelID, ver string
	cmd := &cobra.Command{
		Use: "create", Short: "Create a version",
		RunE: func(_ *cobra.Command, _ []string) error {
			var result map[string]interface{}
			err := client.Post("/api/v1/models/"+modelID+"/versions", map[string]string{"version": ver}, &result)
			if err != nil {
				return err
			}
			PrintSuccess(fmt.Sprintf("Version %s created (status: %s)", result["Version"], result["Status"]))
			fmt.Printf("  ID: %s\n", result["ID"])
			return nil
		},
	}
	cmd.Flags().StringVar(&modelID, "model", "", "Model ID (required)")
	cmd.Flags().StringVar(&ver, "version", "", "Semver version (required)")
	_ = cmd.MarkFlagRequired("model")
	_ = cmd.MarkFlagRequired("version")
	return cmd
}

func newVerPromoteCmd() *cobra.Command {
	var status string
	cmd := &cobra.Command{
		Use: "promote [version-id]", Short: "Promote a version status", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var result map[string]interface{}
			err := client.Patch("/api/v1/versions/"+args[0]+"/status", map[string]string{"status": status}, &result)
			if err != nil {
				return err
			}
			PrintSuccess(fmt.Sprintf("Version promoted to %s", result["Status"]))
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "ACTIVE", "Target status (STAGED, ACTIVE, ARCHIVED)")
	return cmd
}
