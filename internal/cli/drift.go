package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDriftCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "drift", Short: "Drift detection"}
	cmd.AddCommand(newDriftCheckCmd())
	cmd.AddCommand(newDriftStatusCmd())
	return cmd
}

func newDriftCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use: "check [model-version-id]", Short: "Run drift check", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var result []map[string]interface{}
			if err := client.Post("/api/v1/drift/"+args[0]+"/check", nil, &result); err != nil {
				return err
			}
			if outputFmt == "json" {
				PrintJSON(result)
				return nil
			}
			if len(result) == 0 {
				fmt.Println("No drift data available (insufficient telemetry)")
				return nil
			}
			var rows [][]string
			for _, r := range result {
				rows = append(rows, []string{
					fmt.Sprint(r["metric"]), fmt.Sprintf("%.4f", r["psi_value"]),
					fmt.Sprint(r["severity"]), fmt.Sprint(r["message"]),
				})
			}
			PrintTable([]string{"Metric", "PSI", "Severity", "Message"}, rows)
			return nil
		},
	}
}

func newDriftStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use: "status [model-version-id]", Short: "Show latest drift results", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var result []map[string]interface{}
			if err := client.Get("/api/v1/drift/"+args[0], &result); err != nil {
				return err
			}
			if outputFmt == "json" {
				PrintJSON(result)
				return nil
			}
			if len(result) == 0 {
				fmt.Println("No drift results available")
				return nil
			}
			var rows [][]string
			for _, r := range result {
				rows = append(rows, []string{
					fmt.Sprint(r["metric"]), fmt.Sprintf("%.4f", r["psi_value"]),
					fmt.Sprint(r["severity"]),
					fmt.Sprintf("%v", r["baseline_sample"]), fmt.Sprintf("%v", r["current_sample"]),
				})
			}
			PrintTable([]string{"Metric", "PSI", "Severity", "Baseline N", "Current N"}, rows)
			return nil
		},
	}
}
