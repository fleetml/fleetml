package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newExperimentCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "experiment", Short: "Manage A/B experiments"}
	cmd.AddCommand(newExpCreateCmd())
	cmd.AddCommand(newExpListCmd())
	cmd.AddCommand(newExpGetCmd())
	cmd.AddCommand(newExpActionCmd("start", "Start experiment"))
	cmd.AddCommand(newExpActionCmd("pause", "Pause experiment"))
	cmd.AddCommand(newExpActionCmd("resume", "Resume experiment"))
	cmd.AddCommand(newExpActionCmd("cancel", "Cancel experiment"))
	cmd.AddCommand(newExpResultsCmd())
	cmd.AddCommand(newExpConcludeCmd())
	return cmd
}

func newExpCreateCmd() *cobra.Command {
	var projectID, name, control, treatment, metric string
	var split int
	var autoConclude bool
	cmd := &cobra.Command{
		Use: "create", Short: "Create an experiment",
		RunE: func(_ *cobra.Command, _ []string) error {
			body := map[string]interface{}{
				"project_id":           projectID,
				"name":                 name,
				"control_version_id":   control,
				"treatment_version_id": treatment,
				"primary_metric":       metric,
				"split_ratio":          split,
				"auto_conclude":        autoConclude,
			}
			var result map[string]interface{}
			if err := client.Post("/api/v1/experiments", body, &result); err != nil {
				return err
			}
			if outputFmt == "json" {
				PrintJSON(result)
				return nil
			}
			PrintSuccess(fmt.Sprintf("Experiment created: %s (id: %s)", result["name"], result["id"]))
			return nil
		},
	}
	cmd.Flags().StringVar(&projectID, "project", "", "Project ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "Experiment name (required)")
	cmd.Flags().StringVar(&control, "control", "", "Control version ID (required)")
	cmd.Flags().StringVar(&treatment, "treatment", "", "Treatment version ID (required)")
	cmd.Flags().StringVar(&metric, "metric", "latency", "Primary metric (latency, error_rate, confidence)")
	cmd.Flags().IntVar(&split, "split", 50, "Traffic split % to treatment")
	cmd.Flags().BoolVar(&autoConclude, "auto-conclude", false, "Auto-conclude on significance")
	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("control")
	_ = cmd.MarkFlagRequired("treatment")
	return cmd
}

func newExpListCmd() *cobra.Command {
	var projectID string
	cmd := &cobra.Command{
		Use: "list", Short: "List experiments",
		RunE: func(_ *cobra.Command, _ []string) error {
			path := "/api/v1/experiments"
			if projectID != "" {
				path += "?project_id=" + projectID
			}
			var result []map[string]interface{}
			if err := client.Get(path, &result); err != nil {
				return err
			}
			if outputFmt == "json" {
				PrintJSON(result)
				return nil
			}
			var rows [][]string
			for _, e := range result {
				rows = append(rows, []string{
					fmt.Sprint(e["id"]), fmt.Sprint(e["name"]),
					fmt.Sprint(e["status"]), fmt.Sprint(e["primary_metric"]),
				})
			}
			PrintTable([]string{"ID", "Name", "Status", "Metric"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&projectID, "project", "", "Filter by project ID")
	return cmd
}

func newExpGetCmd() *cobra.Command {
	return &cobra.Command{
		Use: "get [experiment-id]", Short: "Get experiment details", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var result map[string]interface{}
			if err := client.Get("/api/v1/experiments/"+args[0], &result); err != nil {
				return err
			}
			PrintJSON(result)
			return nil
		},
	}
}

func newExpActionCmd(action, desc string) *cobra.Command {
	return &cobra.Command{
		Use: action + " [experiment-id]", Short: desc, Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var result map[string]interface{}
			if err := client.Post("/api/v1/experiments/"+args[0]+"/"+action, nil, &result); err != nil {
				return err
			}
			if outputFmt == "json" {
				PrintJSON(result)
				return nil
			}
			PrintSuccess(fmt.Sprintf("Experiment %s → %s", action, result["status"]))
			return nil
		},
	}
}

func newExpResultsCmd() *cobra.Command {
	return &cobra.Command{
		Use: "results [experiment-id]", Short: "Get experiment results", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var result map[string]interface{}
			if err := client.Get("/api/v1/experiments/"+args[0]+"/results", &result); err != nil {
				return err
			}
			PrintJSON(result)
			return nil
		},
	}
}

func newExpConcludeCmd() *cobra.Command {
	var winner, note string
	cmd := &cobra.Command{
		Use: "conclude [experiment-id]", Short: "Conclude experiment with winner", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var result map[string]interface{}
			body := map[string]string{"winner_version_id": winner, "note": note}
			if err := client.Post("/api/v1/experiments/"+args[0]+"/conclude", body, &result); err != nil {
				return err
			}
			PrintSuccess(fmt.Sprintf("Experiment concluded: winner=%s", winner[:12]))
			return nil
		},
	}
	cmd.Flags().StringVar(&winner, "winner", "", "Winner version ID (required)")
	cmd.Flags().StringVar(&note, "note", "", "Conclusion note")
	_ = cmd.MarkFlagRequired("winner")
	return cmd
}
