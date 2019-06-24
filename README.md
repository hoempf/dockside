# Dockside

## Motivation

Using Docker on Windows generally works but lacks one specific feature:

* File system change notifications.

They do not propagate correctly to Docker on Windows because its probably not
implemented in SMB.

This means developing with hot-reload features (like with many JS
libraries/frameworks) on Windows and Docker is not going to work properly.

`Dockside` scans all bind-mounted volumes in Docker containers by querying the
Docker daemon and watches file system change notifications in Windows (the
source file system). It then forwards those change notifications inside the
running containers by issuing a `chmod` to the files changed so `inotify` and
other mechanism inside the (linux) container can act accordingly.

## Why chmod was chosen

`chmod` is not supported by NTFS and therefore will not circle back if used
inside the container.

## How to use

Make sure those environment variables are set correctly (the usually are if you
installed Docker for Windows):

```sh
DOCKER_HOST="tcp://0.0.0.0:2375"
dockside.exe
```

`Dockside` automatically keeps track of containers starting/stopping and watches
all bind-mounts by default.

It coalesces change notifications and sends them off at least every 500ms. It
also works concurrently so the same directory mounted inside multiple containers
will update those containers concurrently. Also multiple notifications for the
same file and container are reduced to a single notification.

## Compile

```sh
GOOS=windows go build
```

**NOTE:** It may compile for Linux but this doesn't make sense, and probably
would result in an infinite loop when forwarding via `chmod`.
