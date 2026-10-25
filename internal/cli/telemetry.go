package cli

import (
	"fmt"
	"time"

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
			versions := map[string]string{} // version ID -> its version, looked up once
			var rows [][]string
			for _, e := range result {
				rows = append(rows, []string{
					eventTime(e["recorded_at"]),
					versionName(versions, fmt.Sprint(e["model_version_id"])),
					fmt.Sprint(e["event_type"]),
					fmt.Sprint(e["latency_ms"]),
					confidence(e["confidence_score"]),
					fmt.Sprint(e["error_message"]),
				})
			}
			PrintTable([]string{"Time", "Version", "Type", "Latency(ms)", "Confidence", "Error"}, rows)
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

// confidence prints a confidence score with 3 decimals: devices send float32,
// so 0.97 arrives as 0.9700000286102295.
// eventTime prints an event's recorded_at in local time, to the second.
func eventTime(v interface{}) string {
	s, _ := v.(string)
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return fmt.Sprint(v)
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

// versionName is the version (1.0.0) of a model version ID, fetched once per
// ID; the ID's first 8 characters if it cannot be read.
func versionName(cache map[string]string, id string) string {
	if v, ok := cache[id]; ok {
		return v
	}
	name := id
	if len(name) > 8 {
		name = name[:8]
	}
	var ver map[string]interface{}
	if err := client.Get("/api/v1/versions/"+id, &ver); err == nil {
		if s, ok := ver["version"].(string); ok && s != "" {
			name = s
		}
	}
	cache[id] = name
	return name
}

func confidence(v interface{}) string {
	if f, ok := v.(float64); ok {
		return fmt.Sprintf("%.3f", f)
	}
	return fmt.Sprint(v)
}
