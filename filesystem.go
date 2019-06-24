package main

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
)

// FileMonitor is an abstraction struct for file system events.
type FileMonitor struct {
	Watcher *fsnotify.Watcher

	onChange OnChangeFunc
}

// Prove FileMonitor is a Closer.
var _ io.Closer = &FileMonitor{}

// NewFileMonitor returns a new file system events monitor.
func NewFileMonitor() (*FileMonitor, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, errors.Wrapf(err, "could not start file system monitoring")
	}
	m := &FileMonitor{
		Watcher: w,
	}
	return m, nil
}

// Close closes the underlying file monitor.
func (m *FileMonitor) Close() error {
	return m.Watcher.Close()
}

// OnChangeFunc is a function called when something in a file system we monitor
// has changed.
type OnChangeFunc func(path string)

// OnChange is called when a file system object changes.
func (m *FileMonitor) OnChange(f OnChangeFunc) {
	m.onChange = f
}

// Start watching for file system notifications.
func (m *FileMonitor) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case event, ok := <-m.Watcher.Events:
				if !ok {
					panic("TODO: implement")
				}
				if event.Op == fsnotify.Remove {
					// File has been removed, so it can't be chmodded anymore.
					// The container doesn't get the delete notification either
					// though. We also can't create the same file and delete it
					// in the container because it would most certainly cause an
					// infinite loop.
					continue
				}
				m.onChange(event.Name)
			case err, ok := <-m.Watcher.Errors:
				if !ok {
					panic("TODO: implement")
				}
				log.Println("error from fsnotify:", err)
			case <-ctx.Done():
				log.Println(ctx.Err())
				return
			}
		}
	}()
}

// PathFromDocker returns a docker path in Docker for Windows where
// `host_mnt/<drive-letter>` is replaced with <drive-letter>:\
func PathFromDocker(path string) string {
	re := regexp.MustCompile(`^/host_mnt/([a-z]+)`)
	path = re.ReplaceAllString(path, `$1:`)
	path = filepath.FromSlash(path)
	return path
}

// PathFromWindows does the opposite of `PathFromDocker`.
func PathFromWindows(path string) string {
	path = filepath.ToSlash(path)
	re := regexp.MustCompile(`^([a-zA-Z]+):`)
	path = re.ReplaceAllString(path, `/host_mnt/$1`)
	return path
}

// Watch does watch a file or a directory. If a directory is passed, it watches
// all its contents, files and subdirectories, recursively.
func (m *FileMonitor) Watch(path string) error {
	path = PathFromDocker(path)
	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.Wrapf(err, "error traversing path %s", path)
		}
		if !info.IsDir() {
			// We don't monitor files directly.
			return nil
		}
		return m.Watcher.Add(path)
	})
	if err != nil {
		return errors.Wrapf(err, "error walking path %s", path)
	}
	return nil
}

// Unwatch stops watching the path. It unwatches all subdirectories and files
// recursively.
func (m *FileMonitor) Unwatch(path string) error {
	path = PathFromDocker(path)
	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.Wrapf(err, "error traversing path %s", path)
		}
		if !info.IsDir() {
			// Since we don't monitor files directly we also don't "unmonitor"
			// them :)
			return nil
		}
		return m.Watcher.Remove(path)
	})
	if err != nil {
		return errors.Wrapf(err, "error walking path %s", path)
	}
	return nil
}
