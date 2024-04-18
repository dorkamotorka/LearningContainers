# runc Container Project

## Description

This project uses `runc`, a CLI tool for spawning and running containers according to the OCI specification.

## Prerequisites

- Tested it on Linux Ubuntu 22.04
- Go 1.21 or lower (runc not working with go 1.22)

## Installation

Install runc from [here](https://github.com/opencontainers/runc)

## Usage

### Creating an OCI Bundle (fancy name for a regular folder)

In order to use runc you must have your container in the format of an OCI bundle.
If you have Docker installed you can use its `export` method to acquire a root filesystem from an existing Docker container.

```bash
# create the top most bundle directory
mkdir /mycontainer
cd /mycontainer

# create the rootfs directory
mkdir rootfs

# export busybox via Docker into the rootfs directory
docker export $(docker create busybox) | tar -C rootfs -xvf -
```

After a root filesystem is populated you just generate a spec in the format of a `config.json` file inside your bundle.
`runc` provides a `spec` command to generate a base template spec that you are then able to edit.
To find features and documentation for fields in the spec please refer to the [specs](https://github.com/opencontainers/runtime-spec) repository.

```bash
runc spec
```

#### Extracting Container Image Filesystem

The problem with the docker export command is that it works with containers and not with their images. An obvious workaround would be to start a container and use the export.

**💡 Pro Tip**: By default, extracting files from a tar archive sets the file ownership to the current user. If the original file ownership needs to be preserved, you can use the `--same-owner` flag while extracting the archive. Beware that you'll have to be sufficiently privileged for that.

Example: 
```bash
sudo tar --same-owner -xf nginx.tar.gz -C rootfs
```

However, running a container just to see its image contents has significant downsides:

- The technique might be unnecessarily slow (e.g., heavy container startup logic).
- Running arbitrary containers is potentially insecure.
- Some files can be modified upon startup, spoiling the export results.
- Sometimes, running a container is simply impossible (e.g., a broken image).

The working combo is to use `docker create` + `docker export`.

The well-known `docker run` command is actually a shortcut for two less frequently used commands - `docker create <IMAGE>` and `docker start <CONTAINER>`. And since containers aren't (only) processes, the `docker create` command, in particular, prepares the root filesystem for the future container.

```
CONT_ID=$(docker create nginx:latest)
docker export ${CONT_ID} -o nginx.tar.gz
```

or together like above (with extraction to the `rootfs` directory):

```
docker export $(docker create busybox) | tar -C rootfs -xvf -
```

### Running Containers

Assuming you have an OCI bundle from the previous step you can execute the container in two different ways.

The first way is to use the convenience command `run` that will handle creating, starting, and deleting the container after it exits.

```bash
# run as root
cd /mycontainer
runc run mycontainerid
```

If you used the unmodified `runc spec` template this should give you a `sh` session inside the container.

The second way to start a container is using the specs lifecycle operations.
This gives you more power over how the container is created and managed while it is running.
This will also launch the container in the background so you will have to edit
the `config.json` to remove the `terminal` setting for the simple examples
below (see more details about [runc terminal handling](docs/terminals.md)).
Your process field in the `config.json` should look like this below with `"terminal": false` and `"args": ["sleep", "5"]`.


```json
        "process": {
                "terminal": false,
                "user": {
                        "uid": 0,
                        "gid": 0
                },
                "args": [
                        "sleep", "5"
                ],
                "env": [
                        "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
                        "TERM=xterm"
                ],
                "cwd": "/",
                "capabilities": {
                        "bounding": [
                                "CAP_AUDIT_WRITE",
                                "CAP_KILL",
                                "CAP_NET_BIND_SERVICE"
                        ],
                        "effective": [
                                "CAP_AUDIT_WRITE",
                                "CAP_KILL",
                                "CAP_NET_BIND_SERVICE"
                        ],
                        "inheritable": [
                                "CAP_AUDIT_WRITE",
                                "CAP_KILL",
                                "CAP_NET_BIND_SERVICE"
                        ],
                        "permitted": [
                                "CAP_AUDIT_WRITE",
                                "CAP_KILL",
                                "CAP_NET_BIND_SERVICE"
                        ],
                        "ambient": [
                                "CAP_AUDIT_WRITE",
                                "CAP_KILL",
                                "CAP_NET_BIND_SERVICE"
                        ]
                },
                "rlimits": [
                        {
                                "type": "RLIMIT_NOFILE",
                                "hard": 1024,
                                "soft": 1024
                        }
                ],
                "noNewPrivileges": true
        },
```

Now we can go through the lifecycle operations in your shell.


```bash
# run as root
cd /mycontainer
runc create mycontainerid

# view the container is created and in the "created" state
runc list

# start the process inside the container
runc start mycontainerid

# after 5 seconds view that the container has exited and is now in the stopped state
runc list

# now delete the container
runc delete mycontainerid
```

**IMPORTANT**: This allows higher level systems to augment the containers creation logic with setup of various settings after the container is created and/or before it is deleted. For example, the container's network stack is commonly set up after `create` but before `start`.

### config.json

The format of the configuration file is defined by the OCI Runtime Specification. It's a JSON document that describes the container's runtime environment, including the process to run inside the container, its arguments, environment variables, namespaces, and so on. Even though it's relatively straightforward, it may get pretty lengthy, so writing it from scratch is not the best idea. Luckily, the `runc spec` command can generate a template configuration file for you.

For more information check [here](https://github.com/opencontainers/runtime-spec/blob/main/spec.md).