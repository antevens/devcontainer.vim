package devcontainer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/mikoto2000/devcontainer.vim/v3/util"
)

// addWatchRecursive registers path and, if it is a directory, all subdirectories
// with watcher so that inotify events fire for the entire tree.
func addWatchRecursive(watcher *fsnotify.Watcher, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return watcher.Add(path)
	}
	return filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi == nil {
			return nil
		}
		if fi.IsDir() {
			return watcher.Add(p)
		}
		return nil
	})
}

// watchDereferencedMounts uses inotify (via fsnotify) to watch the source paths
// of dereferenced mounts and re-copies them into the container the moment a change
// is detected. A 200 ms debounce coalesces bursts of events (e.g. a git pull)
// into a single docker-cp. The function runs until ctx is cancelled, which happens
// when the vim session ends.
func watchDereferencedMounts(ctx context.Context, containerID, containerHome string, mounts []map[string]interface{}) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create file watcher: %v\n", err)
		return
	}
	defer watcher.Close()

	type mountEntry struct {
		sourceRaw string
		targetRaw string
		realPath  string
		dirty     chan struct{}
	}

	var entries []mountEntry

	for _, mount := range mounts {
		sourceRaw, okS := mount["source"].(string)
		targetRaw, okT := mount["target"].(string)
		if !okS || !okT {
			continue
		}

		sourceExpanded, err := util.ExtractShellVariables(strings.ReplaceAll(sourceRaw, "${localEnv:HOME}", "$HOME"))
		if err != nil {
			sourceExpanded = sourceRaw
		}
		sourceExpanded = strings.TrimSpace(sourceExpanded)

		realPath, err := filepath.EvalSymlinks(sourceExpanded)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: cannot resolve %s for watching: %v\n", sourceExpanded, err)
			continue
		}

		if err := addWatchRecursive(watcher, realPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to watch %s: %v\n", realPath, err)
			continue
		}

		// Buffered channel of size 1 acts as a coalescing dirty flag: multiple
		// rapid sends before the consumer wakes up collapse into one sync.
		dirty := make(chan struct{}, 1)
		entry := mountEntry{
			sourceRaw: sourceRaw,
			targetRaw: targetRaw,
			realPath:  realPath,
			dirty:     dirty,
		}
		entries = append(entries, entry)

		go func(e mountEntry) {
			for {
				select {
				case <-ctx.Done():
					return
				case <-e.dirty:
					// Debounce: drain additional signals for 200 ms so a burst of
					// changes (e.g. writing several credential fields) triggers one copy.
					timer := time.NewTimer(200 * time.Millisecond)
				drain:
					for {
						select {
						case <-e.dirty:
							timer.Reset(200 * time.Millisecond)
						case <-timer.C:
							break drain
						case <-ctx.Done():
							timer.Stop()
							return
						}
					}
					// Write to the terminal title bar instead of the screen so we
					// don't corrupt vim's full-screen rendering. OSC 2 updates the
					// window title without touching the character-cell buffer.
					fmt.Fprintf(os.Stderr, "\033]2;devcontainer.vim: syncing %s\007", filepath.Base(e.sourceRaw))
					syncErr := transferDereferencedMount(containerID, containerHome, e.sourceRaw, e.targetRaw)
					if syncErr != nil {
						fmt.Fprintf(os.Stderr, "\033]2;devcontainer.vim: sync failed: %s\007", filepath.Base(e.sourceRaw))
					} else {
						fmt.Fprintf(os.Stderr, "\033]2;devcontainer.vim: sync done: %s\007", filepath.Base(e.sourceRaw))
					}
					// Reset title after a short delay so vim can reassert its own.
					time.AfterFunc(3*time.Second, func() {
						fmt.Fprintf(os.Stderr, "\033]2;\007")
					})
				}
			}
		}(entry)
	}

	if len(entries) == 0 {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			eventPath := filepath.Clean(event.Name)
			for i, e := range entries {
				rootClean := filepath.Clean(e.realPath)
				if eventPath == rootClean || strings.HasPrefix(eventPath, rootClean+string(filepath.Separator)) {
					// Signal dirty — non-blocking so we never block the event loop.
					select {
					case entries[i].dirty <- struct{}{}:
					default:
					}

					// If a new subdirectory was created, watch it too so we catch
					// future changes inside it.
					if event.Has(fsnotify.Create) {
						if fi, err := os.Stat(event.Name); err == nil && fi.IsDir() {
							_ = addWatchRecursive(watcher, event.Name)
						}
					}
					break
				}
			}
		case watchErr, ok := <-watcher.Errors:
			if !ok {
				return
			}
			fmt.Fprintf(os.Stderr, "Warning: file watcher error: %v\n", watchErr)
		}
	}
}
