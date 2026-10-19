package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newEnrollmentKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enrollment-key",
		Short: "Manage a project's device enrollment keys",
		Long: "Devices, the Linux agent and the SDKs register into a project with one of its\n" +
			"enrollment keys. The key itself is printed once, when it is created.",
	}
	cmd.AddCommand(newEnrollmentKeyCreateCmd())
	cmd.AddCommand(newEnrollmentKeyListCmd())
	return cmd
}

func newEnrollmentKeyCreateCmd() *cobra.Command {
	var projectID, name string
	cmd := &cobra.Command{
		Use: "create", Short: "Create an enrollment key and print it once",
		RunE: func(_ *cobra.Command, _ []string) error {
			var result map[string]interface{}
			err := client.Post("/api/v1/projects/"+projectID+"/enrollment-keys", map[string]string{"name": name}, &result)
			if err != nil {
				return err
			}
			if outputFmt == "json" {
				PrintJSON(result)
				return nil
			}
			PrintSuccess(fmt.Sprintf("Enrollment key created: %s", result["name"]))
			fmt.Printf("  ID:  %s\n", result["id"])
			fmt.Printf("  Key: %s\n", result["key"])
			fmt.Println("  The key is not shown again; store it now.")
			return nil
		},
	}
	cmd.Flags().StringVar(&projectID, "project", "", "Project ID (required)")
	cmd.Flags().StringVar(&name, "name", "default", "Key name")
	_ = cmd.MarkFlagRequired("project")
	return cmd
}

func newEnrollmentKeyListCmd() *cobra.Command {
	var projectID string
	cmd := &cobra.Command{
		Use: "list", Short: "List a project's enrollment keys (without the keys)",
		RunE: func(_ *cobra.Command, _ []string) error {
			var result []map[string]interface{}
			if err := client.Get("/api/v1/projects/"+projectID+"/enrollment-keys", &result); err != nil {
				return err
			}
			if outputFmt == "json" {
				PrintJSON(result)
				return nil
			}
			var rows [][]string
			for _, k := range result {
				rows = append(rows, []string{
					fmt.Sprint(k["id"]), fmt.Sprint(k["name"]), fmt.Sprint(k["created_at"]),
					orDash(k["last_used_at"]), orDash(k["revoked_at"]),
				})
			}
			PrintTable([]string{"ID", "Name", "Created", "Last used", "Revoked"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&projectID, "project", "", "Project ID (required)")
	_ = cmd.MarkFlagRequired("project")
	return cmd
}

// orDash prints a nullable field, "-" for null.
func orDash(v interface{}) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprint(v)
}
