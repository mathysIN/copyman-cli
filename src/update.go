package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/manifoldco/promptui"
)

const (
	githubRepo = "mathysin/copyman-cli"
	githubAPI  = "https://api.github.com/repos/" + githubRepo
)

type Release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// version is set at build time via ldflags
var version = "dev"

func getCurrentVersion() string {
	if version == "" || version == "dev" {
		return "dev"
	}
	return version
}

func checkForUpdate() (*Release, error) {
	resp, err := http.Get(githubAPI + "/releases/latest")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

func isUpdateAvailable(current, latest string) bool {
	if current == "dev" {
		return false
	}
	// Simple version comparison (removes 'v' prefix)
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")
	return current != latest
}

func getDownloadURL(release *Release) string {
	os := runtime.GOOS
	arch := runtime.GOARCH

	var suffix string
	switch os {
	case "linux":
		if arch == "arm64" {
			suffix = "linux-arm64"
		} else {
			suffix = "linux-amd64"
		}
	case "darwin":
		if arch == "arm64" {
			suffix = "darwin-arm64"
		} else {
			suffix = "darwin-amd64"
		}
	case "windows":
		suffix = "windows-amd64"
	}

	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, suffix) {
			return asset.BrowserDownloadURL
		}
	}
	return ""
}

func runUpdate(args []string) error {
	currentVersion := getCurrentVersion()

	fmt.Printf("Current version: %s\n", currentVersion)
	fmt.Println("Checking for updates...")

	release, err := checkForUpdate()
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	latestVersion := release.TagName

	if !isUpdateAvailable(currentVersion, latestVersion) {
		fmt.Println("✅ You're already on the latest version!")
		return nil
	}

	fmt.Printf("\n🎉 New version available: %s (current: %s)\n", latestVersion, currentVersion)
	fmt.Printf("Release notes: %s\n\n", release.HTMLURL)

	// Show download URL
	downloadURL := getDownloadURL(release)
	if downloadURL != "" {
		fmt.Printf("Download: %s\n\n", downloadURL)
	}

	// Prompt for auto-update
	prompt := promptui.Prompt{
		Label:     "Would you like to update now",
		IsConfirm: true,
	}

	result, err := prompt.Run()
	if err != nil || strings.ToLower(result) != "y" {
		fmt.Println("Update cancelled. You can manually download from:")
		fmt.Printf("  %s\n", release.HTMLURL)
		return nil
	}

	// Perform update
	return performUpdate(release)
}

func performUpdate(release *Release) error {
	downloadURL := getDownloadURL(release)
	if downloadURL == "" {
		return fmt.Errorf("no binary available for your platform (%s/%s)", runtime.GOOS, runtime.GOARCH)
	}

	fmt.Println("Downloading update...")

	// Download new binary
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	// Get current executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Create temp file
	tmpFile := execPath + ".tmp"
	f, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	// Copy downloaded content
	_, err = f.ReadFrom(resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to write update: %w", err)
	}

	// Replace old binary
	backupPath := execPath + ".backup"

	// Backup current binary
	if err := os.Rename(execPath, backupPath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Move new binary to place
	if err := os.Rename(tmpFile, execPath); err != nil {
		// Restore backup on failure
		os.Rename(backupPath, execPath)
		return fmt.Errorf("failed to install update: %w", err)
	}

	// Remove backup
	os.Remove(backupPath)

	fmt.Printf("\n✅ Successfully updated to %s!\n", release.TagName)
	fmt.Println("Run 'copyman --version' to verify.")

	return nil
}

func runVersion() {
	fmt.Printf("copyman version %s\n", getCurrentVersion())
	fmt.Printf("Built for: %s/%s\n", runtime.GOOS, runtime.GOARCH)
}
