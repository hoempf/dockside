package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/docker/docker/api/types/filters"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
)

// Container is a short struct describing a container and its name.
type Container struct {
	ID   string
	Name string
}

// Dockwatch interfaces with Docker API.
type Dockwatch struct {
	Client *client.Client // The Docker API client.
	list   []Container    // A list of containers to watch and update.
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
	w := &Dockwatch{
		Client: cli,
		list:   make([]Container, 0),
		ctx:    ctx,
	}
	args := filters.NewArgs()
	args.Add("type", "container")
	w.events, w.errors = cli.Events(ctx, types.EventsOptions{
		Filters: args,
	})

	return w, nil
}

func (d *Dockwatch) String() string {
	var buf strings.Builder
	for k, v := range d.list {
		buf.WriteString(fmt.Sprintf(
			"[%d] %s %s\n",
			k,
			v.ID,
			v.Name,
		))
	}
	return buf.String()
}

// WatchContainer keeps an internally updated list of running containers. It
// watches Docker events.
func (d *Dockwatch) WatchContainer() error {
	ctx := d.ctx
	if len(d.list) == 0 {
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
				d.handleEvent(ev)
			case err := <-d.errors:
				d.handleErrors(err)
			case <-ctx.Done():
				// We're done here
				return
			}
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
		d.addToList(ev.Actor.ID, ev.Actor.Attributes["name"])
	case "stop", "kill", "die", "pause":
		d.removeFromList(ev.Actor.ID)
	}

	fmt.Printf("list: %+v\n", d.list)
}

// addToList add a container id to the watched list.
func (d *Dockwatch) addToList(containerID string, name string) {
	d.cmux.Lock()
	defer d.cmux.Unlock()

	for _, v := range d.list {
		if v.ID == containerID {
			return
		}
	}

	d.list = append(d.list, Container{
		ID:   containerID,
		Name: name,
	})
}

func (d *Dockwatch) removeFromList(containerID string) {
	d.cmux.Lock()
	defer d.cmux.Unlock()

	list := make([]Container, len(d.list)-1)

	var i int
	for _, v := range d.list {
		if v.ID == containerID {
			continue
		}
		list[i] = v
		i++
	}
	d.list = list
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
		d.list = append(d.list, Container{
			ID:   v.ID,
			Name: strings.TrimSpace(strings.Join(v.Names, " ")),
		})
	}

	return nil
}
