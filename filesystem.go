package main

import (
	"io"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
)

// FileMonitor is an abstraction struct for file system events.
type FileMonitor struct {
	Watcher *fsnotify.Watcher
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

// Start watching file system change events.
func (m *FileMonitor) Start() {

}

// Watch does watch a file or a directory. If a directory is passed, it watches
// all its contents, files and subdirectories, recursively.
func (m *FileMonitor) Watch(path string) error {
	return nil
}
