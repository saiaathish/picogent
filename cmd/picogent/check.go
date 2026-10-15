package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/saiaathish/picogent/internal/agyauth"
	"github.com/saiaathish/picogent/internal/verify"
	"os"
)

func runConnectionCheck(ctx context.Context, args []string) error {
	if len(args) >= 1 && args[0] == "project" {
		workspace, err := os.Getwd()
		if err != nil {
			return err
		}
		if len(args) == 3 && args[1] == "--dir" {
			workspace = args[2]
		} else if len(args) != 1 {
			return fmt.Errorf("usage: picogent check project [--dir PATH]")
		}
		result := verify.RunPipeline(ctx, workspace, verify.Options{})
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			return err
		}
		if result.Status != verify.StatusPass {
			return fmt.Errorf("project checks: %s", result.Status)
		}
		return nil
	}
	if len(args) != 1 || args[0] != "antigravity" {
		return fmt.Errorf("usage: picogent check antigravity")
	}
	report := agyauth.CheckConnection(ctx)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return err
	}
	if !report.OK {
		return fmt.Errorf("Antigravity connection not verified; see the checks above")
	}
	return nil
}
