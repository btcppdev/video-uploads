package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bitcoinplusplus/btcpp-video/internal/uploader"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context
	mgr *uploader.Manager
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	mgr, err := uploader.New(func(snapshot uploader.Snapshot) {
		runtime.EventsEmit(ctx, "upload:snapshot", snapshot)
	})
	if err != nil {
		runtime.LogErrorf(ctx, "uploader startup: %v", err)
		return
	}
	a.mgr = mgr
	a.mgr.Start(ctx)
}

func (a *App) shutdown(context.Context) {
	if a.mgr != nil {
		a.mgr.Stop()
	}
}

func (a *App) Snapshot() uploader.Snapshot {
	if a.mgr == nil {
		return uploader.Snapshot{}
	}
	return a.mgr.Snapshot()
}

func (a *App) AddFiles(paths []string, destination uploader.Destination) (uploader.Snapshot, error) {
	if a.mgr == nil {
		return uploader.Snapshot{}, fmt.Errorf("upload service is not ready")
	}
	return a.mgr.AddFiles(paths, destination)
}

func (a *App) SelectVideoFiles(destination uploader.Destination) (uploader.Snapshot, error) {
	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Choose rough video files",
		Filters: []runtime.FileFilter{{DisplayName: "Video files", Pattern: "*.mp4;*.mov;*.mxf;*.mkv;*.mts;*.m2ts;*.avi;*.wav"}},
	})
	if err != nil {
		return a.Snapshot(), err
	}
	if len(paths) == 0 {
		return a.Snapshot(), nil
	}
	return a.AddFiles(paths, destination)
}

func (a *App) Pause(id string) uploader.Snapshot  { return a.mgr.Pause(id) }
func (a *App) Resume(id string) uploader.Snapshot { return a.mgr.Resume(id) }
func (a *App) Retry(id string) uploader.Snapshot  { return a.mgr.Resume(id) }
func (a *App) Remove(id string) uploader.Snapshot { return a.mgr.Remove(id) }
func (a *App) PauseAll() uploader.Snapshot        { return a.mgr.PauseAll() }
func (a *App) ResumeAll() uploader.Snapshot       { return a.mgr.ResumeAll() }
func (a *App) ClearCompleted() uploader.Snapshot  { return a.mgr.ClearCompleted() }

func (a *App) Settings() uploader.Settings                   { return a.mgr.Settings() }
func (a *App) SaveSettings(settings uploader.Settings) error { return a.mgr.SaveSettings(settings) }

func (a *App) DetectRemovableMedia() []string {
	roots := []string{"/Volumes", "/media", "/run/media"}
	var out []string
	for _, root := range roots {
		entries, _ := os.ReadDir(root)
		for _, entry := range entries {
			if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
				out = append(out, filepath.Join(root, entry.Name()))
			}
		}
	}
	return out
}
