package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newProjectCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "project", Short: "Manage projects"}
	cmd.AddCommand(newProjectListCmd())
	cmd.AddCommand(newProjectCreateCmd())
	cmd.AddCommand(newProjectGetCmd())
	cmd.AddCommand(newProjectDeleteCmd())
	return cmd
}

func newProjectListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List all projects",
		RunE: func(_ *cobra.Command, _ []string) error {
			var result []map[string]interface{}
			if err := client.Get("/api/v1/projects", &result); err != nil {
				return err
			}
			if outputFmt == "json" {
				PrintJSON(result)
				return nil
			}
			var rows [][]string
			for _, p := range result {
				rows = append(rows, []string{
					fmt.Sprint(p["id"]), fmt.Sprint(p["name"]), fmt.Sprint(p["slug"]),
				})
			}
			PrintTable([]string{"ID", "Name", "Slug"}, rows)
			return nil
		},
	}
}

func newProjectCreateCmd() *cobra.Command {
	var name, desc string
	cmd := &cobra.Command{
		Use: "create", Short: "Create a project",
		RunE: func(_ *cobra.Command, _ []string) error {
			var result map[string]interface{}
			err := client.Post("/api/v1/projects", map[string]string{"name": name, "description": desc}, &result)
			if err != nil {
				return err
			}
			if outputFmt == "json" {
				PrintJSON(result)
				return nil
			}
			PrintSuccess(fmt.Sprintf("Project created: %s (slug: %s)", result["name"], result["slug"]))
			fmt.Printf("  ID: %s\n", result["id"])
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Project name (required)")
	_ = cmd.MarkFlagRequired("name")
	cmd.Flags().StringVar(&desc, "description", "", "Project description")
	return cmd
}

func newProjectGetCmd() *cobra.Command {
	return &cobra.Command{
		Use: "get [project-id]", Short: "Get project details", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var result map[string]interface{}
			if err := client.Get("/api/v1/projects/"+args[0], &result); err != nil {
				return err
			}
			if outputFmt == "json" {
				PrintJSON(result)
				return nil
			}
			PrintDetail("Project", [][2]string{
				{"ID", fmt.Sprint(result["id"])},
				{"Name", fmt.Sprint(result["name"])},
				{"Slug", fmt.Sprint(result["slug"])},
			})
			return nil
		},
	}
}

func newProjectDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use: "delete [project-id]", Short: "Delete a project", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := client.Delete("/api/v1/projects/" + args[0]); err != nil {
				return err
			}
			PrintSuccess("Project deleted")
			return nil
		},
	}
}
