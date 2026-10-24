package cli

import (
	"encoding/json"
	"fmt"
	"strings"

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
		Use:   "delete [project-id]",
		Short: "Delete a project and everything in it",
		Long: "Delete a project with its models, versions, deployments, experiments,\n" +
			"enrollment keys and devices, and the telemetry of its versions.",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var project map[string]interface{}
			if err := client.Get("/api/v1/projects/"+args[0], &project); err != nil {
				return err
			}
			contents := projectContents(args[0])
			if err := client.Delete("/api/v1/projects/" + args[0]); err != nil {
				return err
			}
			if outputFmt == "json" {
				PrintJSON(map[string]interface{}{"deleted": project["id"], "removed": contents})
				return nil
			}
			var parts []string
			for _, c := range contents {
				parts = append(parts, plural(c.Count, c.Name))
			}
			PrintSuccess(fmt.Sprintf("Project %s deleted, with %s and their telemetry", project["name"], strings.Join(parts, ", ")))
			return nil
		},
	}
}

// projectCount is how many of one kind of thing a project holds.
type projectCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// projectContents counts what deleting a project removes, from the list
// routes' totals. A count it cannot read is left out.
func projectContents(id string) []projectCount {
	total := func(path string) (int, bool) {
		raw, err := client.GetRaw(path)
		if err != nil {
			return 0, false
		}
		var resp struct {
			Data       json.RawMessage `json:"data"`
			Pagination *struct {
				Total int `json:"total"`
			} `json:"pagination"`
		}
		if json.Unmarshal(raw, &resp) != nil {
			return 0, false
		}
		if resp.Pagination != nil {
			return resp.Pagination.Total, true
		}
		var items []json.RawMessage // a list without pagination
		if json.Unmarshal(resp.Data, &items) != nil {
			return 0, false
		}
		return len(items), true
	}
	var out []projectCount
	add := func(name, path string) {
		if n, ok := total(path); ok {
			out = append(out, projectCount{name, n})
		}
	}
	var models []map[string]interface{}
	if err := client.Get("/api/v1/projects/"+id+"/models?limit=1000", &models); err == nil {
		out = append(out, projectCount{"model", len(models)})
		versions := 0
		for _, m := range models {
			n, _ := total(fmt.Sprintf("/api/v1/models/%v/versions?limit=1", m["id"]))
			versions += n
		}
		out = append(out, projectCount{"version", versions})
	}
	add("deployment", "/api/v1/deployments?limit=1&project_id="+id)
	add("experiment", "/api/v1/experiments?limit=1&project_id="+id)
	add("device", "/api/v1/devices?limit=1&project_id="+id)
	add("enrollment key", "/api/v1/projects/"+id+"/enrollment-keys")
	return out
}

func plural(n int, name string) string {
	if n == 1 {
		return "1 " + name
	}
	return fmt.Sprintf("%d %ss", n, name)
}
