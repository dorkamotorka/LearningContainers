Before running do:

Install e.g. Alpine image (mini-rootfs) from (here)[https://alpinelinux.org/downloads/] and extract it to `new_root` using:

```
tar -xf alpine-minirootfs-3.19.1-x86_64.tar -C new_root/
```

Then do:

```
sudo unshare -m --propagation private
```

This command will create a new mount namespace and set this root mount to private.
This way we suffice the needs of the root_pivot, that required both old root and new root to NOT be MS_SHARED!