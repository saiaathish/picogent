package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/saiaathish/picogent/internal/agyauth"
	"os"
)

func runConnectionCheck(ctx context.Context, args []string) error {
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
