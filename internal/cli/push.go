package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func newPushCmd() *cobra.Command {
	var projectID, modelName, ver, quant, framework, task string
	var promote bool

	cmd := &cobra.Command{
		Use:   "push [file]",
		Short: "Upload a model variant (create project/model/version if needed)",
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
			fmt.Printf("Preparing %s (%dKB, sha256:%s)...\n", filePath, fileSize/1024, sha[:12])
			var model map[string]interface{}
			err = client.Post("/api/v1/projects/"+projectID+"/models", map[string]string{
				"name": modelName, "framework": framework, "task_type": task,
			}, &model)
			if err != nil {
				// Try to find existing
				var models []map[string]interface{}
				if err2 := client.Get("/api/v1/projects/"+projectID+"/models", &models); err2 != nil {
					return fmt.Errorf("creating model: %s", err)
				}
				for _, m := range models {
					if fmt.Sprint(m["Name"]) == modelName {
						model = m
						break
					}
				}
				if model == nil {
					return fmt.Errorf("creating model: %s", err)
				}
			}
			modelID := fmt.Sprint(model["ID"])

			// Step 2: Ensure version exists
			var version map[string]interface{}
			err = client.Post("/api/v1/models/"+modelID+"/versions", map[string]string{"version": ver}, &version)
			if err != nil {
				var versions []map[string]interface{}
				if err2 := client.Get("/api/v1/models/"+modelID+"/versions", &versions); err2 != nil {
					return fmt.Errorf("creating version: %s", err)
				}
				for _, v := range versions {
					if fmt.Sprint(v["Version"]) == ver {
						version = v
						break
					}
				}
				if version == nil {
					return fmt.Errorf("creating version: %s", err)
				}
			}
			versionID := fmt.Sprint(version["ID"])

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
			variantID := fmt.Sprint(variant["ID"])

			// Step 4: Upload with progress
			f, err = os.Open(filePath)
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()

			pw := &ProgressWriter{Total: fileSize}
			reader := io.TeeReader(f, pw)
			if err := client.PutRaw(uploadURL, reader, fileSize); err != nil {
				return fmt.Errorf("upload failed: %w", err)
			}
			fmt.Fprintln(os.Stderr) // newline after progress bar

			// Step 5: Finalize
			var finalResp map[string]interface{}
			if err := client.Post("/api/v1/variants/"+variantID+"/finalize", nil, &finalResp); err != nil {
				return fmt.Errorf("finalize failed: %s", err)
			}

			// Step 6: Promote if requested
			if promote {
				// DRAFT -> STAGED -> ACTIVE
				var stageResp map[string]interface{}
				_ = client.Patch("/api/v1/versions/"+versionID+"/status", map[string]string{"status": "STAGED"}, &stageResp)
				_ = client.Patch("/api/v1/versions/"+versionID+"/status", map[string]string{"status": "ACTIVE"}, &stageResp)
			}

			PrintSuccess(fmt.Sprintf("Pushed %s → %s v%s (%s)", filePath, modelName, ver, quant))
			fmt.Printf("  Variant ID: %s\n", variantID)
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
	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("model")
	_ = cmd.MarkFlagRequired("version")
	return cmd
}
