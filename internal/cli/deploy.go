package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func newDeployCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "deploy", Short: "Manage deployments"}
	cmd.AddCommand(newDeployCreateCmd())
	cmd.AddCommand(newDeployListCmd())
	cmd.AddCommand(newDeployGetCmd())
	cmd.AddCommand(newDeployActionCmd("start", "Start a deployment rollout"))
	cmd.AddCommand(newDeployActionCmd("pause", "Pause a deployment"))
	cmd.AddCommand(newDeployActionCmd("resume", "Resume a paused deployment"))
	cmd.AddCommand(newDeployProgressCmd())
	cmd.AddCommand(newDeployRollbackCmd())
	cmd.AddCommand(newDeployHealthCmd())
	return cmd
}

func newDeployCreateCmd() *cobra.Command {
	var versionID, strategy, target, healthGate, canaryStages, rollingConfig, projectID string
	var urgent bool
	cmd := &cobra.Command{
		Use: "create", Short: "Create a deployment",
		RunE: func(_ *cobra.Command, _ []string) error {
			body := map[string]interface{}{
				"model_version_id": versionID,
				"strategy":         strategy,
				"urgent":           urgent,
			}
			if projectID != "" {
				body["project_id"] = projectID
			}
			if target != "" {
				var ts map[string]interface{}
				if err := json.Unmarshal([]byte(target), &ts); err != nil {
					return fmt.Errorf("invalid --target JSON: %w", err)
				}
				body["target_selector"] = ts
			}
			if healthGate != "" {
				var hg map[string]interface{}
				if err := json.Unmarshal([]byte(healthGate), &hg); err != nil {
					return fmt.Errorf("invalid --health-gate JSON: %w", err)
				}
				body["health_gate"] = hg
			}
			if canaryStages != "" {
				var cs []interface{}
				if err := json.Unmarshal([]byte(canaryStages), &cs); err != nil {
					return fmt.Errorf("invalid --canary-stages JSON: %w", err)
				}
				body["canary_stages"] = cs
			}
			if rollingConfig != "" {
				var rc map[string]interface{}
				if err := json.Unmarshal([]byte(rollingConfig), &rc); err != nil {
					return fmt.Errorf("invalid --rolling-config JSON: %w", err)
				}
				body["rolling_config"] = rc
			}
			var result map[string]interface{}
			if err := client.Post("/api/v1/deployments", body, &result); err != nil {
				return err
			}
			if outputFmt == "json" {
				PrintJSON(result)
				return nil
			}
			PrintSuccess(fmt.Sprintf("Deployment created: %s (strategy: %s)", result["id"], result["strategy"]))
			return nil
		},
	}
	cmd.Flags().StringVar(&versionID, "version", "", "Model version ID (required)")
	cmd.Flags().StringVar(&projectID, "project", "", "Project ID (required)")
	cmd.Flags().StringVar(&strategy, "strategy", "ALL_AT_ONCE", "Strategy (ALL_AT_ONCE, CANARY, ROLLING)")
	cmd.Flags().StringVar(&target, "target", "", "Target selector JSON")
	cmd.Flags().StringVar(&healthGate, "health-gate", "", "Health gate JSON")
	cmd.Flags().StringVar(&canaryStages, "canary-stages", "", `Canary stages JSON, e.g. [{"percentage":25,"min_duration":"10m"},{"percentage":100,"min_duration":"0s"}]`)
	cmd.Flags().StringVar(&rollingConfig, "rolling-config", "", `Rolling config JSON, e.g. {"batch_size_percent":20,"batch_interval":"1m"}`)
	cmd.Flags().BoolVar(&urgent, "urgent", false, "Devices download on any network, ignoring their download policy")
	_ = cmd.MarkFlagRequired("version")
	_ = cmd.MarkFlagRequired("project")
	return cmd
}

func newDeployListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List deployments",
		RunE: func(_ *cobra.Command, _ []string) error {
			var result []map[string]interface{}
			if err := client.Get("/api/v1/deployments", &result); err != nil {
				return err
			}
			if outputFmt == "json" {
				PrintJSON(result)
				return nil
			}
			var rows [][]string
			for _, d := range result {
				rows = append(rows, []string{
					fmt.Sprint(d["id"]), fmt.Sprint(d["strategy"]),
					fmt.Sprint(d["status"]), fmt.Sprint(d["current_stage"]),
				})
			}
			PrintTable([]string{"ID", "Strategy", "Status", "Stage"}, rows)
			return nil
		},
	}
}

func newDeployGetCmd() *cobra.Command {
	return &cobra.Command{
		Use: "get [deployment-id]", Short: "Get deployment details", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var result map[string]interface{}
			if err := client.Get("/api/v1/deployments/"+args[0], &result); err != nil {
				return err
			}
			PrintJSON(result)
			return nil
		},
	}
}

func newDeployActionCmd(action, desc string) *cobra.Command {
	return &cobra.Command{
		Use: action + " [deployment-id]", Short: desc, Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var result map[string]interface{}
			if err := client.Post("/api/v1/deployments/"+args[0]+"/"+action, nil, &result); err != nil {
				return err
			}
			if outputFmt == "json" {
				PrintJSON(result)
				return nil
			}
			PrintSuccess(fmt.Sprintf("Deployment %s → %s", action, result["status"]))
			return nil
		},
	}
}

func newDeployProgressCmd() *cobra.Command {
	return &cobra.Command{
		Use: "progress [deployment-id]", Short: "Advance to next stage", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var result map[string]interface{}
			if err := client.Post("/api/v1/deployments/"+args[0]+"/progress", nil, &result); err != nil {
				return err
			}
			if outputFmt == "json" {
				PrintJSON(result)
				return nil
			}
			PrintSuccess(fmt.Sprintf("Progressed → stage %v, status: %s", result["current_stage"], result["status"]))
			return nil
		},
	}
}

func newDeployRollbackCmd() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use: "rollback [deployment-id]", Short: "Rollback deployment", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var result map[string]interface{}
			body := map[string]string{"reason": reason}
			if err := client.Post("/api/v1/deployments/"+args[0]+"/rollback", body, &result); err != nil {
				return err
			}
			PrintSuccess(fmt.Sprintf("Rolled back: %s", result["rollback_reason"]))
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "manual rollback", "Rollback reason")
	return cmd
}

func newDeployHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use: "health [deployment-id]", Short: "Check health gate status", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var result map[string]interface{}
			if err := client.Get("/api/v1/deployments/"+args[0]+"/health", &result); err != nil {
				return err
			}
			PrintJSON(result)
			return nil
		},
	}
}
