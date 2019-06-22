package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
)

// Container is a short struct describing a container and its name.
type Container struct {
	ID   string
	Name string

	Volumes Volumes
}

func (c *Container) String() string {
	return fmt.Sprintf(
		"{ID: %s.., Name: %s}",
		c.ID[:8],
		c.Name,
	)
}

// Equals returns true if the container has the same ID.
func (c *Container) Equals(other *Container) bool {
	if c.ID == other.ID {
		return true
	}
	return false
}

// ContainerList is a list of Containers.
type ContainerList struct {
	list []*Container // Container list.
	mux  sync.RWMutex // Protects this list.
}

// NewContainerList returns a new empty list.
func NewContainerList() *ContainerList {
	list := &ContainerList{
		list: make([]*Container, 0),
	}
	return list
}

func (l *ContainerList) String() string {
	ss := make([]string, len(l.list))
	for k, v := range l.list {
		ss[k] = v.String()
	}
	return strings.TrimSpace(strings.Join(ss, " "))
}

// Upsert inserts a container to the list in case it doesn't exist. If it does
// exist it updates the item in the list.
func (l *ContainerList) Upsert(c *Container) {
	l.mux.Lock()
	defer l.mux.Unlock()
	for k, v := range l.list {
		if v.Equals(c) {
			l.list[k] = c
			return
		}
	}
	l.list = append(l.list, c)
}

// Remove container with specified ID.
func (l *ContainerList) Remove(id string) {
	l.mux.Lock()
	defer l.mux.Unlock()
	// Get index of container by ID.
	for k, v := range l.list {
		if v.ID != id {
			continue
		}
		// Order is not important so we can swap the last element with the one
		// removed and shorten the slice.
		l.list[k] = l.list[len(l.list)-1]
		l.list = l.list[:len(l.list)-1]
		return
	}
	// Remove is idempotent.
}

// Len returns the lenght of the list.
func (l *ContainerList) Len() int {
	l.mux.RLock()
	defer l.mux.RUnlock()
	return len(l.list)
}

// Volumes is a list of Volumes.
type Volumes []*Volume

// Volume is a container volume.
type Volume struct {
	ID      string
	SrcPath string
	DstPath string
}

// Dockwatch interfaces with Docker API.
type Dockwatch struct {
	Client *client.Client // The Docker API client.
	list   *ContainerList // A list of containers to watch and update.
	cmux   sync.RWMutex   // Protects Containers.

	ctx    context.Context
	events <-chan events.Message
	errors <-chan error
}

// NewDockwatch returns a new Dockwatch instance with a Docker API client.
func NewDockwatch(ctx context.Context) (*Dockwatch, error) {
	cli, err := client.NewEnvClient()
	if err != nil {
		return nil, errors.Wrap(err, "could not create docker api client")
	}
	d := &Dockwatch{
		Client: cli,
		list:   NewContainerList(),
		ctx:    ctx,
	}

	d.initEventListeners()
	return d, nil
}

// initEventListeners listens to Docker daemon events.
func (d *Dockwatch) initEventListeners() {
	args := filters.NewArgs()
	args.Add("type", "container")
	d.events, d.errors = d.Client.Events(d.ctx, types.EventsOptions{
		Filters: args,
	})
	log.Println("event listeners started")
}

func (d *Dockwatch) String() string {
	return d.list.String()
}

// WatchContainer keeps an internally updated list of running containers. It
// watches Docker events.
func (d *Dockwatch) WatchContainer() error {
	ctx := d.ctx
	if d.list.Len() == 0 {
		// We started just now so the list is empty. Get all containers running
		// now and matching the name.
		if err := d.resetContainerList(); err != nil {
			return errors.Wrap(err, "could not reset container list")
		}
	}

	// Watch docker events for container creation / stopping so we have an
	// accurate list.
	go func() {
		for {
			select {
			case ev := <-d.events:
				log.Printf("got event from Docker: %s %s", ev.Action, ev.Actor.ID)
				d.handleEvent(ev)
			case err := <-d.errors:
				if err == io.EOF {
					// Reinit the event stream.
					log.Printf("got EOF on error channel of docker events: %v", err)
					d.initEventListeners()
				}
				// Handle other errors.
				d.handleErrors(err)
			case <-ctx.Done():
				return
			}

			log.Println(d.list)
		}
	}()

	return nil
}

// handleEvent .
func (d *Dockwatch) handleEvent(ev events.Message) {
	if ev.Type != events.ContainerEventType {
		// This should not happen because we filter by container types, but just
		// in case.
		log.Printf("event type %s received, expected %s", ev.Type, events.ContainerEventType)
		return
	}

	switch ev.Action {
	case "start", "unpause":
		d.list.Upsert(&Container{
			ID:   ev.Actor.ID,
			Name: ev.Actor.Attributes["name"],
		})
	case "stop", "kill", "die", "pause", "destroy":
		d.list.Remove(ev.Actor.ID)
	}
}

// handleErrors .
func (d *Dockwatch) handleErrors(err error) {
	fmt.Printf("%v\n", err)
}

// resetContainerList updates the internal list. It deletes all current entries
// and fetches a fresh list from the Docker daemon.
func (d *Dockwatch) resetContainerList() error {
	d.cmux.Lock()
	defer d.cmux.Unlock()

	list, err := d.Client.ContainerList(d.ctx, types.ContainerListOptions{})
	if err != nil {
		return errors.Wrap(err, "could not fetch fresh container list")
	}

	for _, v := range list {
		d.list.Upsert(&Container{
			ID:   v.ID,
			Name: strings.TrimSpace(strings.Join(v.Names, " ")),
		})
	}

	log.Println("reset container list:")
	log.Println(d.list)
	return nil
}
