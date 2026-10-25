package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func newPushCmd() *cobra.Command {
	var projectID, modelName, ver, quant, framework, task, metadata string
	var promote bool

	cmd := &cobra.Command{
		Use:   "push [file]",
		Short: "Upload a model variant (create the model and version if needed)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			filePath := args[0]

			// Validate file
			fi, err := os.Stat(filePath)
			if err != nil {
				return fmt.Errorf("file not found: %s", filePath)
			}
			fileSize := fi.Size()

			// Compute SHA256
			f, err := os.Open(filePath)
			if err != nil {
				return fmt.Errorf("opening file: %w", err)
			}
			h := sha256.New()
			if _, err := io.Copy(h, f); err != nil {
				_ = f.Close()
				return fmt.Errorf("reading file: %w", err)
			}
			_ = f.Close()
			sha := hex.EncodeToString(h.Sum(nil))

			// Step 1: Ensure model exists
			fmt.Printf("Preparing %s (%s, sha256:%s)...\n", filePath, sizeString(fileSize), sha[:12])
			var model map[string]interface{}
			err = client.Post("/api/v1/projects/"+projectID+"/models", map[string]string{
				"name": modelName, "framework": framework, "task_type": task,
			}, &model)
			if err != nil {
				// Try to find existing, a page of 100 at a time
				for offset := 0; model == nil; offset += 100 {
					var models []map[string]interface{}
					if err2 := client.Get(fmt.Sprintf("/api/v1/projects/%s/models?limit=100&offset=%d", projectID, offset), &models); err2 != nil {
						return fmt.Errorf("creating model: %s", err)
					}
					for _, m := range models {
						if fmt.Sprint(m["name"]) == modelName {
							model = m
							break
						}
					}
					if len(models) < 100 {
						break
					}
				}
				if model == nil {
					return fmt.Errorf("creating model: %s", err)
				}
			}
			modelID := fmt.Sprint(model["id"])

			// Step 2: Ensure version exists
			versionBody := map[string]interface{}{"version": ver}
			if metadata != "" {
				var md map[string]interface{}
				if err := json.Unmarshal([]byte(metadata), &md); err != nil {
					return fmt.Errorf("invalid --metadata JSON: %w", err)
				}
				versionBody["metadata"] = md
			}
			var version map[string]interface{}
			err = client.Post("/api/v1/models/"+modelID+"/versions", versionBody, &version)
			if err != nil {
				// The existing version, a page of 100 at a time.
				for offset := 0; version == nil; offset += 100 {
					var versions []map[string]interface{}
					if err2 := client.Get(fmt.Sprintf("/api/v1/models/%s/versions?limit=100&offset=%d", modelID, offset), &versions); err2 != nil {
						return fmt.Errorf("creating version: %s", err)
					}
					for _, v := range versions {
						if fmt.Sprint(v["version"]) == ver {
							version = v
							break
						}
					}
					if len(versions) < 100 {
						break
					}
				}
				if version == nil {
					return fmt.Errorf("creating version: %s", err)
				}
				if metadata != "" {
					fmt.Fprintf(os.Stderr, "Warning: version %s already exists; --metadata applies only when push creates the version\n", ver)
				}
			}
			versionID := fmt.Sprint(version["id"])

			// Step 3: Create variant + get upload URL
			var variantResp map[string]interface{}
			err = client.Post("/api/v1/versions/"+versionID+"/variants", map[string]interface{}{
				"quantization":    quant,
				"target_runtime":  framework,
				"blob_size_bytes": fileSize,
				"blob_sha256":     sha,
			}, &variantResp)
			if err != nil {
				return fmt.Errorf("creating variant: %s", err)
			}

			uploadURL := fmt.Sprint(variantResp["upload_url"])
			variant := variantResp["variant"].(map[string]interface{})
			variantID := fmt.Sprint(variant["id"])

			// Step 4: Upload with progress
			f, err = os.Open(filePath)
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()

			// The progress bar redraws one line with \r: only on a terminal.
			var reader io.Reader = f
			progress := isTerminal(os.Stderr)
			if progress {
				reader = io.TeeReader(f, &ProgressWriter{Total: fileSize})
			}
			if err := client.PutRaw(uploadURL, reader, fileSize); err != nil {
				return err // "upload failed: ..."
			}
			if progress {
				fmt.Fprintln(os.Stderr) // newline after progress bar
			}

			// Step 5: Finalize
			var finalResp map[string]interface{}
			if err := client.Post("/api/v1/variants/"+variantID+"/finalize", nil, &finalResp); err != nil {
				return fmt.Errorf("finalize failed: %s", err)
			}

			// Step 6: Promote if requested: DRAFT -> STAGED -> ACTIVE from where
			// the version is now, so a second variant of an ACTIVE version
			// changes nothing. The file is uploaded either way, so a failed
			// promotion says what the version is left at.
			alreadyActive := false
			if promote {
				var current map[string]interface{}
				if err := client.Get("/api/v1/versions/"+versionID, &current); err != nil {
					return fmt.Errorf("uploaded %s as %s v%s (variant %s), but reading its status failed: %w", filePath, modelName, ver, variantID, err)
				}
				steps := []string{"STAGED", "ACTIVE"} // DRAFT; ARCHIVED is refused by the server
				switch fmt.Sprint(current["status"]) {
				case "STAGED":
					steps = []string{"ACTIVE"}
				case "ACTIVE":
					steps, alreadyActive = nil, true
				}
				for _, status := range steps {
					var stageResp map[string]interface{}
					if err := client.Patch("/api/v1/versions/"+versionID+"/status", map[string]string{"status": status}, &stageResp); err != nil {
						return fmt.Errorf("uploaded %s as %s v%s (variant %s), but promoting it to %s failed: %w", filePath, modelName, ver, variantID, status, err)
					}
				}
			}

			PrintSuccess(fmt.Sprintf("Pushed %s → %s v%s (%s)", filePath, modelName, ver, quant))
			fmt.Printf("  Variant ID: %s\n", variantID)
			if alreadyActive {
				fmt.Printf("  v%s was already ACTIVE\n", ver)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "Project ID (required)")
	cmd.Flags().StringVar(&modelName, "model", "", "Model name (required)")
	cmd.Flags().StringVar(&ver, "version", "", "Semver version (required)")
	cmd.Flags().StringVar(&quant, "quantization", "INT8", "Quantization (FP32, FP16, INT8, INT4, DYNAMIC)")
	cmd.Flags().StringVar(&framework, "framework", "TFLITE", "Framework/runtime")
	cmd.Flags().StringVar(&task, "task", "CLASSIFICATION", "Task type")
	cmd.Flags().BoolVar(&promote, "promote", false, "Promote version to ACTIVE after upload")
	cmd.Flags().StringVar(&metadata, "metadata", "", `Version metadata JSON for a new version, e.g. {"fleetml_requirements":{"min_ram_mb":64,"min_score":0}}`)
	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("model")
	_ = cmd.MarkFlagRequired("version")
	return cmd
}

// sizeString is a file size for people: bytes below 1 KB, then KB and MB.
func sizeString(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d bytes", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}
