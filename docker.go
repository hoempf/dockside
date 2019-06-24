package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
)

// Container is a short struct describing a container and its name.
type Container struct {
	ID   string
	Name string

	Mounts []*Mount
}

func (c *Container) String() string {
	return fmt.Sprintf(
		"{ID: %s.., Name: %s, Mounts: %v}",
		c.ID[:8],
		c.Name,
		c.Mounts,
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
// exist it updates the item in the list. It returns the container
// inserted/updated and true if the element already existed and has been
// updated, false otherwise.
func (l *ContainerList) Upsert(c *Container) (*Container, bool) {
	l.mux.Lock()
	defer l.mux.Unlock()
	for k, v := range l.list {
		if v.Equals(c) {
			l.list[k] = c
			return c, true
		}
	}
	l.list = append(l.list, c)
	return c, false
}

// Remove container with specified ID. It returns the container which has been
// removed, nil if the container didn't exist.
func (l *ContainerList) Remove(id string) *Container {
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
		return v
	}
	// Remove is idempotent.
	return nil
}

// Len returns the lenght of the list.
func (l *ContainerList) Len() int {
	l.mux.RLock()
	defer l.mux.RUnlock()
	return len(l.list)
}

// Reset clears the list without deallocating memory.
func (l *ContainerList) Reset() {
	l.mux.Lock()
	defer l.mux.Unlock()
	l.list = l.list[:0]
}

// Walk through the list and execute f for each item encountered. It stops
// walking if the func returns an error.
func (l *ContainerList) Walk(f func(*Container) error) error {
	for _, v := range l.list {
		if err := f(v); err != nil {
			return err
		}
	}
	return nil
}

// Mount is a bind-mount into a container.
type Mount struct {
	SrcPath string // Source path on the host.
	DstPath string // Destination bind-mount path in the container.
}

func (m *Mount) String() string {
	return fmt.Sprintf("{src:%s, dst:%s}", m.SrcPath, m.DstPath)
}

// OnStartFunc is a function called when a container is started/unpaused etc.
type OnStartFunc func(*Container)

// OnStopFunc is a function called when a container is stopped etc.
type OnStopFunc func(*Container)

// Dockwatch interfaces with Docker API.
type Dockwatch struct {
	Client *client.Client // The Docker API client.

	onStart OnStartFunc // Callback when containers start.
	onStop  OnStopFunc  // Callback when containers stop.

	list   *ContainerList // A list of containers to watch and update.
	ctx    context.Context
	events <-chan events.Message
	errors <-chan error
	work   chan *Event            // Work channel for new events.
	queues map[string]chan *Event // Queues for each container.
	mux    sync.Mutex             // Protects queues.
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
		work:   make(chan *Event),
		queues: make(map[string]chan *Event),
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

// OnStart sets the callback function called when containers are started. We
// make sure it is called only once per state transition.
func (d *Dockwatch) OnStart(f OnStartFunc) {
	d.onStart = f
}

// OnStop sets the callback function called when containers are stopped. We make
// sure it is called only once per state transition. E.g. `stop` followed by
// `destroy` will only call this once.
func (d *Dockwatch) OnStop(f OnStopFunc) {
	d.onStop = f
}

// Start the event goroutines with n workers.
func (d *Dockwatch) Start(n int) {
	out := make(chan *Event)
	log.Printf("starting %d workers", n)
	for i := 0; i < n; i++ {
		go func() {
			for ev := range out {
				err := d.executeChmod(ev)
				if err != nil {
					log.Printf("could not execute chmod inside container %s: %v", ev.Container.ID, err)
				}
			}
		}()
	}

	go func() {
		defer close(out)
		for ev := range d.work {
			d.mux.Lock()
			cid := ev.Container.ID
			qu, ok := d.queues[cid]
			if !ok {
				// Setup a new queue and attach out channel.
				qu = make(chan *Event)
				go d.coalesce(qu, out)
				d.queues[cid] = qu
			}
			d.mux.Unlock()
			qu <- ev
		}
	}()
}

// coalesce buffers Events for a short time. It tries to update multiple
// files/dirs at once inside containers without having to launch a `docker exec`
// process for each file. It also filters repeated change notification of the
// same file during one cycle.
func (d *Dockwatch) coalesce(in <-chan *Event, out chan<- *Event) {
	timer := time.NewTimer(0) // Fires immediately, but we're going to reset it.
	var (
		timerCh <-chan time.Time
		outCh   chan<- *Event
		event   *Event
		oldEv   *Event
	)
	for {
		var err error
		select {
		case ev := <-in:
			if event == nil {
				event = ev
			} else {
				oldEv = event
				event, err = event.Merge(ev)
			}
			if err != nil {
				// Fire immediately and put the new event in front of the buffer.
				// In theory this should not happen.
				out <- oldEv
				event = ev
			}

			if timerCh == nil {
				timer.Reset(500 * time.Millisecond) //TODO: Make this configurable.
				timerCh = timer.C
			}
		case <-timerCh:
			outCh = out
			timerCh = nil // This skips the channel during the next evaluation of `case`.
		case outCh <- event:
			event = nil
			outCh = nil // This skips the channel during the next evaluation of `case`.
		}
	}
}

// WatchContainer keeps an internally updated list of running containers. It
// watches Docker events.
func (d *Dockwatch) WatchContainer() error {
	ctx := d.ctx
	if d.list.Len() == 0 {
		// We started just now so the list is empty. Get all containers running
		// now.
		if err := d.resetContainerList(); err != nil {
			return errors.Wrap(err, "could not reset container list")
		}
	}

	// Watch docker events for container creation / stopping so we have an
	// accurate list.
	go func() {
		defer close(d.work)
		for {
			select {
			case ev := <-d.events:
				log.Printf("got event from Docker: %s %s", ev.Action, ev.Actor.ID)
				d.handleEvent(ev)
			case err := <-d.errors:
				if err == io.EOF {
					// The stream disconnected. Consider this a fatal error.
					// Trying to recreate a connection to a dead daemon would
					// spin endlessly.
					log.Fatalf("got EOF on error channel of docker events: %v", err)
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

// ForwardChange proxies the file system notification event into the container
// by issuing `chmod <path>` (docker exec). It does not `touch` because that
// would end up in an infinite loop.
func (d *Dockwatch) ForwardChange(path string) error {
	err := d.list.Walk(func(c *Container) error {
		for _, v := range c.Mounts {
			p := strings.Replace(PathFromWindows(path), v.SrcPath, "", 1)
			if p == "" {
				continue
			}
			dst := v.DstPath + p
			ev := &Event{
				Container: c,
				Files:     []string{dst},
			}
			select {
			case d.work <- ev:
			case <-d.ctx.Done():
				return d.ctx.Err()
			}
		}
		return nil
	})
	return err
}

func (d *Dockwatch) executeChmod(e *Event) error {
	// Prepare the "docker exec" command.
	cmd := []string{
		"sh",
		"-c",
		fmt.Sprintf(`for i in %s; do chmod $(stat -c %%a $i) $i; done`, strings.Join(e.Files, " ")),
	}
	execCfg := types.ExecConfig{
		Cmd:          cmd,
		Tty:          false,
		AttachStdout: true,
		AttachStderr: true,
	}
	id, err := d.Client.ContainerExecCreate(d.ctx, e.Container.ID, execCfg)
	if err != nil {
		return errors.Wrapf(err, "cannot create exec in container %s", e.Container.ID)
	}

	// This timeout for the execution should be more than enough. If
	// this takes longer we can assume there's a deeper problem.
	ctx, cancel := context.WithTimeout(d.ctx, time.Minute)
	defer cancel()

	// During execution of the command attach a reader.
	resp, err := d.Client.ContainerExecAttach(d.ctx, id.ID, execCfg)
	if err != nil {
		return errors.Wrapf(err, "cannot connect stderr/stdout to exec process")
	}
	defer resp.Close()

	// Execute the "docker exec" command.
	err = d.Client.ContainerExecStart(ctx, id.ID, types.ExecStartCheck{})
	if err != nil {
		return errors.Wrapf(err, "cannot start exec inside container %s", e.Container.ID)
	}

	// Read back what happened.
	scanner := bufio.NewScanner(resp.Reader)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}

	return nil
}

// Event is an update to the file system which needs to be propagated into the
// container.
type Event struct {
	Container *Container
	Files     []string
}

// NewEvent creates a new event.
func NewEvent() *Event {
	e := &Event{
		Files: make([]string, 0),
	}
	return e
}

// Merge two events. Returns an error if the events could not be merged.
func (e *Event) Merge(other *Event) (*Event, error) {
	if !e.Container.Equals(other.Container) {
		return nil, fmt.Errorf("events cannot be merged, containers don't match: %s %s", e.Container, other.Container)
	}

	ev := NewEvent()
	ev.Container = e.Container
	ev.Files = append(e.Files, other.Files...)
	// We only need unique files in one single batch.
	ev.Files = sortUnique(ev.Files)

	return ev, nil
}

// sortUnique does in-place deduplication of a []string. See
// https://github.com/golang/go/wiki/SliceTricks
func sortUnique(in []string) []string {
	sort.Strings(in)
	j := 0
	for i := 1; i < len(in); i++ {
		if in[j] == in[i] {
			continue
		}
		j++
		in[j] = in[i]
	}
	return in[:j+1]
}

// Contains returns true if an event with the same target file exists. It also
// check for container equality. If not equal it returns false because a file
// with the same name could exist in different containers.
func (e *Event) Contains(other *Event) bool {
	if !e.Container.Equals(other.Container) {
		return false
	}
	for _, v1 := range other.Files {
		for _, v2 := range e.Files {
			if v1 == v2 {
				return true
			}
		}
	}
	return false
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
		d.containerStarted(ev.Actor.ID, ev.Actor.Attributes["name"])
	case "stop", "kill", "die", "pause", "destroy":
		if old := d.list.Remove(ev.Actor.ID); old != nil && d.onStop != nil {
			d.mux.Lock()
			defer d.mux.Unlock()
			d.onStop(old)
			// Also remove the queue from the list.
			log.Printf("remove %s from queue list", ev.Actor.ID)
			delete(d.queues, ev.Actor.ID)
		}
	}
}

// handleErrors .
func (d *Dockwatch) handleErrors(err error) {
	fmt.Printf("%v\n", err)
}

func (d *Dockwatch) containerStarted(containerID, name string) {
	mounts, err := d.getMounts(d.ctx, containerID)
	if err != nil {
		log.Printf("could not inspect mounts: %v", err)
		mounts = make([]*Mount, 0)
	}
	c, updated := d.list.Upsert(&Container{
		ID:     containerID,
		Name:   name,
		Mounts: mounts,
	})
	if !updated && d.onStart != nil {
		d.onStart(c)
	}
}

// resetContainerList updates the internal list. It deletes all current entries
// and fetches a fresh list from the Docker daemon.
func (d *Dockwatch) resetContainerList() error {
	d.list.Reset()
	list, err := d.Client.ContainerList(d.ctx, types.ContainerListOptions{})
	if err != nil {
		return errors.Wrap(err, "could not fetch fresh container list")
	}

	for _, v := range list {
		d.containerStarted(v.ID, strings.Join(v.Names, " "))
	}

	log.Println("reset container list:")
	log.Println(d.list)
	return nil
}

// getMounts .
func (d *Dockwatch) getMounts(ctx context.Context, containerID string) ([]*Mount, error) {
	mounts := make([]*Mount, 0)
	mm, err := d.Client.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, errors.Wrapf(err, "could not inspect container %s", containerID)
	}
	for _, v := range mm.Mounts {
		if v.Type == mount.TypeBind && v.RW {
			// We only consider read/write bind-mounts.
			mounts = append(mounts, &Mount{
				SrcPath: v.Source,
				DstPath: v.Destination,
			})
		}
	}
	return mounts, nil
}
