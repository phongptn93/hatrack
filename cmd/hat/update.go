package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/baodq97/hatrack/internal/update"
)

// printVersion prints the version, and the latest release when it is newer. Being offline is
// not an error: the version is still printed.
func printVersion() error {
	fmt.Println("hat", version)
	if update.Off() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if latest, err := update.Cached(ctx, updateCache(), time.Hour); err == nil && update.Newer(version, latest) {
		fmt.Printf("%s is out: run `hat update`\n", latest)
	}
	return nil
}

// selfUpdate replaces hat, and on Windows the tray app beside it, with the latest release. A
// running tray notices its exe changed and restarts into the new one.
func selfUpdate(args []string) error {
	force := len(args) > 0 && args[0] == "--force"
	if version == "dev" && !force {
		return errors.New("this hat was built from source: update it with git pull and go build, or `hat update --force`")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	latest, err := update.Latest(ctx)
	if err != nil {
		return err
	}
	if !force && !update.Newer(version, latest) {
		fmt.Println("hat", version, "is the latest")
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	files := map[string]string{exe: update.Asset("hat", runtime.GOOS, runtime.GOARCH)}
	if runtime.GOOS == "windows" {
		tray := filepath.Join(filepath.Dir(exe), "hatrack-tray.exe")
		if _, err := os.Stat(tray); err == nil {
			files[tray] = update.Asset("hatrack-tray", runtime.GOOS, runtime.GOARCH)
		}
	}
	fmt.Printf("updating %s to %s…\n", version, latest)
	if err := update.Install(ctx, latest, files); err != nil {
		return err
	}
	fmt.Println("updated to", latest)
	return nil
}

func updateCache() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hatrack", "update.json")
}
