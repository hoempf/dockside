// Dockside is a file system change watcher which propagates those changes into
// volumes mounted in Docker for Windows automatically.
package main

import (
	"context"
	"flag"
	"log"
	"sync"
)

func main() {
	var workersFlag int
	flag.IntVar(&workersFlag, "-n", 10, "number of parallel workers to start")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d, err := NewDockwatch(ctx)
	if err != nil {
		log.Fatalf("could not create new dockwatch: %v", err)
	}

	fs, err := NewFileMonitor()
	if err != nil {
		log.Fatalf("could not start monitoring file system: %v", err)
	}
	defer fs.Close()
	fs.OnChange(func(path string) {
		log.Println("file changed", path)
		d.ForwardChange(path)
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := fs.Start(ctx)
		if err != nil {
			log.Printf("error from file system watcher: %v", err)
		}
	}()

	d.OnStart(func(c *Container) {
		log.Println("container started", c)
		// Get the mounts and start watching them.
		for _, mount := range c.Mounts {
			if err := fs.Watch(mount.SrcPath); err != nil {
				log.Printf("error watching path %s: %v", mount.SrcPath, err)
			}
		}
	})
	d.OnStop(func(c *Container) {
		log.Println("container stopped", c)
		// Unwatch the mounts of this container.
		for _, mount := range c.Mounts {
			if err := fs.Unwatch(mount.SrcPath); err != nil {
				log.Printf("error unwatching path %s: %v", mount.SrcPath, err)
			}
		}
	})
	d.Start(workersFlag)

	if err := d.WatchContainer(); err != nil {
		log.Fatalf("could not watch Docker daemon: %v", err)
	}

	wg.Wait()
}
