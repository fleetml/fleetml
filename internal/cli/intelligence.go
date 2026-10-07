package cli

import (
	"github.com/spf13/cobra"
)

func newPreviewCmd() *cobra.Command {
	var deviceID, deploymentID string
	cmd := &cobra.Command{
		Use: "preview", Short: "Preview variant selection for a device + deployment",
		RunE: func(_ *cobra.Command, _ []string) error {
			var result map[string]interface{}
			err := client.Post("/api/v1/intelligence/preview", map[string]string{
				"device_id": deviceID, "deployment_id": deploymentID,
			}, &result)
			if err != nil {
				return err
			}
			PrintJSON(result)
			return nil
		},
	}
	cmd.Flags().StringVar(&deviceID, "device", "", "Device ID (required)")
	cmd.Flags().StringVar(&deploymentID, "deployment", "", "Deployment ID (required)")
	_ = cmd.MarkFlagRequired("device")
	_ = cmd.MarkFlagRequired("deployment")
	return cmd
}
