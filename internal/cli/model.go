package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newModelCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "model", Short: "Manage models"}
	cmd.AddCommand(newModelListCmd())
	cmd.AddCommand(newModelCreateCmd())
	cmd.AddCommand(newModelGetCmd())
	return cmd
}

func newModelListCmd() *cobra.Command {
	var projectID string
	cmd := &cobra.Command{
		Use: "list", Short: "List models in a project",
		RunE: func(_ *cobra.Command, _ []string) error {
			var result []map[string]interface{}
			if err := client.Get("/api/v1/projects/"+projectID+"/models", &result); err != nil {
				return err
			}
			if outputFmt == "json" {
				PrintJSON(result)
				return nil
			}
			var rows [][]string
			for _, m := range result {
				rows = append(rows, []string{fmt.Sprint(m["ID"]), fmt.Sprint(m["Name"]), fmt.Sprint(m["Framework"]), fmt.Sprint(m["TaskType"])})
			}
			PrintTable([]string{"ID", "Name", "Framework", "TaskType"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&projectID, "project", "", "Project ID (required)")
	_ = cmd.MarkFlagRequired("project")
	return cmd
}

func newModelCreateCmd() *cobra.Command {
	var projectID, name, framework, task string
	cmd := &cobra.Command{
		Use: "create", Short: "Create a model",
		RunE: func(_ *cobra.Command, _ []string) error {
			var result map[string]interface{}
			err := client.Post("/api/v1/projects/"+projectID+"/models", map[string]string{
				"name": name, "framework": framework, "task_type": task,
			}, &result)
			if err != nil {
				return err
			}
			if outputFmt == "json" {
				PrintJSON(result)
				return nil
			}
			PrintSuccess(fmt.Sprintf("Model created: %s", result["Name"]))
			fmt.Printf("  ID: %s\n", result["ID"])
			return nil
		},
	}
	cmd.Flags().StringVar(&projectID, "project", "", "Project ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "Model name (required)")
	cmd.Flags().StringVar(&framework, "framework", "TFLITE", "Framework (TFLITE, ONNX, COREML, GGUF, CUSTOM)")
	cmd.Flags().StringVar(&task, "task", "CLASSIFICATION", "Task type")
	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newModelGetCmd() *cobra.Command {
	return &cobra.Command{
		Use: "get [model-id]", Short: "Get model details", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var result map[string]interface{}
			if err := client.Get("/api/v1/models/"+args[0], &result); err != nil {
				return err
			}
			PrintJSON(result)
			return nil
		},
	}
}
