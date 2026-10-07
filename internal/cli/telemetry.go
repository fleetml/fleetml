package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newTelemetryCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "telemetry", Short: "Query telemetry data"}
	cmd.AddCommand(newTelemetryEventsCmd())
	cmd.AddCommand(newTelemetrySummaryCmd())
	return cmd
}

func newTelemetryEventsCmd() *cobra.Command {
	var modelVersionID, deviceID string
	cmd := &cobra.Command{
		Use: "events", Short: "Query telemetry events",
		RunE: func(_ *cobra.Command, _ []string) error {
			path := "/api/v1/telemetry/events?"
			if modelVersionID != "" {
				path += "model_version_id=" + modelVersionID + "&"
			}
			if deviceID != "" {
				path += "device_id=" + deviceID + "&"
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
					fmt.Sprint(e["event_type"]),
					fmt.Sprint(e["latency_ms"]),
					fmt.Sprint(e["confidence_score"]),
					fmt.Sprint(e["error_message"]),
				})
			}
			PrintTable([]string{"Type", "Latency(ms)", "Confidence", "Error"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&modelVersionID, "model-version", "", "Model version ID")
	cmd.Flags().StringVar(&deviceID, "device", "", "Device ID")
	return cmd
}

func newTelemetrySummaryCmd() *cobra.Command {
	var byClass bool
	cmd := &cobra.Command{
		Use: "summary [model-version-id]", Short: "Get telemetry summary", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			path := "/api/v1/telemetry/summary/" + args[0]
			if byClass {
				path += "/by-class"
			}
			var result interface{}
			if err := client.Get(path, &result); err != nil {
				return err
			}
			PrintJSON(result)
			return nil
		},
	}
	cmd.Flags().BoolVar(&byClass, "by-class", false, "Break down by device class")
	return cmd
}
