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
					fmt.Sprint(d["ID"]), fmt.Sprint(d["OS"]),
					fmt.Sprint(d["RAMMB"]), fmt.Sprint(d["Chipset"]),
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
			fmt.Printf("  Class:     %v\n", result["DeviceClass"])
			fmt.Printf("  Score:     %v/100\n", result["Score"])
			fmt.Printf("  OS:        %v\n", result["OS"])
			if mem, ok := result["Memory"].(map[string]interface{}); ok {
				fmt.Printf("  RAM:       %v MB\n", mem["TotalRAMMB"])
			}
			if acc, ok := result["Accelerators"].(map[string]interface{}); ok {
				fmt.Printf("  NPU:       %v\n", acc["HasNPU"])
				fmt.Printf("  GPU:       %v\n", acc["HasGPU"])
				fmt.Printf("  Runtimes:  %v\n", acc["SupportedRuntimes"])
			}
			return nil
		},
	}
}
