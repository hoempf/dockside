// Dockside is a quickly wrote file system change watcher which propagates those
// changes into volumes mounted in Docker for Windows automatically.
package main

import (
	"context"
	"log"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	defer cancel()
	d, err := NewDockwatch(ctx)
	if err != nil {
		log.Fatalf("could create new dockwatch: %v", err)
	}

	if err := d.WatchContainer(); err != nil {
		log.Fatalf("could watch Docker daemon: %v", err)
	}

	time.Sleep(time.Minute * 15)
}
