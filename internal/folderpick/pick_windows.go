//go:build windows

package folderpick

import (
	"fmt"
	"os/exec"
	"strings"
)

func Choose(prompt string) (string, error) {
	if prompt == "" {
		prompt = "Select a project folder"
	}
	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = '%s'
$d.ShowNewFolderButton = $true
if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { $d.SelectedPath }
`, escapePS(prompt))
	out, err := exec.Command("powershell", "-NoProfile", "-STA", "-Command", script).CombinedOutput()
	if err != nil {
		return "", ErrCancelled
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", ErrCancelled
	}
	return path, nil
}

func escapePS(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
