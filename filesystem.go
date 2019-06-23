package main

import "github.com/fsnotify/fsnotify"

// Filesystem is an abstraction struct for file system events.
type Filesystem struct {
	Watchers []*fsnotify.Watcher
}
