//go:build linux

package folderpick

import (
	"errors"
	"os/exec"
	"strings"
)

func Choose(prompt string) (string, error) {
	if prompt == "" {
		prompt = "Select a project folder"
	}
	if path, err := zenity(prompt); err == nil {
		return path, nil
	}
	if path, err := kdialog(prompt); err == nil {
		return path, nil
	}
	return "", errors.New("install zenity or kdialog for folder selection")
}

func zenity(prompt string) (string, error) {
	out, err := exec.Command("zenity", "--file-selection", "--directory", "--title", prompt).Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
			return "", ErrCancelled
		}
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", ErrCancelled
	}
	return path, nil
}

func kdialog(prompt string) (string, error) {
	out, err := exec.Command("kdialog", "--getexistingdirectory", ".", "--title", prompt).Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
			return "", ErrCancelled
		}
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", ErrCancelled
	}
	return path, nil
}
