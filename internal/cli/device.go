package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDeviceCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "device", Short: "Manage devices"}
	cmd.AddCommand(newDeviceListCmd())
	cmd.AddCommand(newDeviceGetCmd())
	cmd.AddCommand(newDeviceProfileCmd())
	return cmd
}

func newDeviceListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List registered devices",
		RunE: func(_ *cobra.Command, _ []string) error {
			var result []map[string]interface{}
			if err := client.Get("/api/v1/devices", &result); err != nil {
				return err
			}
			if outputFmt == "json" {
				PrintJSON(result)
				return nil
			}
			var rows [][]string
			for _, d := range result {
				rows = append(rows, []string{
					fmt.Sprint(d["id"]), fmt.Sprint(d["os"]),
					fmt.Sprint(d["ram_mb"]), fmt.Sprint(d["chipset"]),
				})
			}
			PrintTable([]string{"ID", "OS", "RAM(MB)", "Chipset"}, rows)
			return nil
		},
	}
}

func newDeviceGetCmd() *cobra.Command {
	return &cobra.Command{
		Use: "get [device-id]", Short: "Get device details", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var result map[string]interface{}
			if err := client.Get("/api/v1/devices/"+args[0], &result); err != nil {
				return err
			}
			PrintJSON(result)
			return nil
		},
	}
}

func newDeviceProfileCmd() *cobra.Command {
	return &cobra.Command{
		Use: "profile [device-id]", Short: "Show device capability profile", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var result map[string]interface{}
			if err := client.Get("/api/v1/devices/"+args[0]+"/profile", &result); err != nil {
				return err
			}
			if outputFmt == "json" {
				PrintJSON(result)
				return nil
			}
			fmt.Printf("Device Profile: %s\n", args[0][:12])
			fmt.Println("────────────────────────────")
			fmt.Printf("  Class:     %v\n", result["device_class"])
			fmt.Printf("  Score:     %v/100\n", result["score"])
			fmt.Printf("  OS:        %v\n", result["os"])
			if mem, ok := result["memory"].(map[string]interface{}); ok {
				fmt.Printf("  RAM:       %v MB\n", mem["total_ram_mb"])
			}
			if acc, ok := result["accelerators"].(map[string]interface{}); ok {
				fmt.Printf("  NPU:       %v\n", acc["has_npu"])
				fmt.Printf("  GPU:       %v\n", acc["has_gpu"])
				fmt.Printf("  Runtimes:  %v\n", acc["supported_runtimes"])
			}
			return nil
		},
	}
}
