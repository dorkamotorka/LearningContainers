package main

import (
	"os"
	"os/exec"
	"log"
	"syscall"
	"strconv"
	"golang.org/x/sys/unix"
)

// pivotRoot will call pivot_root such that rootfs becomes the new root
// filesystem, and everything else is cleaned up.
func pivotRoot(rootfs string) error {
	// While the documentation may claim otherwise, pivot_root(".", ".") is
	// actually valid. What this results in is / being the new root but
	// /proc/self/cwd being the old root. Since we can play around with the cwd
	// with pivot_root this allows us to pivot without creating directories in
	// the rootfs. Shout-outs to the LXC developers for giving us this idea.

	oldroot, err := unix.Open("/", unix.O_DIRECTORY|unix.O_RDONLY, 0)
	if err != nil {
		return &os.PathError{Op: "open", Path: "/", Err: err}
	}
	defer unix.Close(oldroot) //nolint: errcheck

	newroot, err := unix.Open(rootfs, unix.O_DIRECTORY|unix.O_RDONLY, 0)
	if err != nil {
		return &os.PathError{Op: "open", Path: rootfs, Err: err}
	}
	defer unix.Close(newroot) //nolint: errcheck

	// Change to the new root so that the pivot_root actually acts on it.
	if err := unix.Fchdir(newroot); err != nil {
		return &os.PathError{Op: "fchdir", Path: "fd " + strconv.Itoa(newroot), Err: err}
	}

	if err := unix.PivotRoot(".", "."); err != nil {
		return &os.PathError{Op: "pivot_root", Path: ".", Err: err}
	}

	// Currently our "." is oldroot (according to the current kernel code).
	// However, purely for safety, we will fchdir(oldroot) since there isn't
	// really any guarantee from the kernel what /proc/self/cwd will be after a
	// pivot_root(2).

	if err := unix.Fchdir(oldroot); err != nil {
		return &os.PathError{Op: "fchdir", Path: "fd " + strconv.Itoa(oldroot), Err: err}
	}

	// Make oldroot rslave to make sure our unmounts don't propagate to the
	// host (and thus bork the machine). We don't use rprivate because this is
	// known to cause issues due to races where we still have a reference to a
	// mount while a process in the host namespace are trying to operate on
	// something they think has no mounts (devicemapper in particular).
	if err := mount("", ".", "", unix.MS_SLAVE|unix.MS_REC, ""); err != nil {
		return err
	}
	// Perform the unmount. MNT_DETACH allows us to unmount /proc/self/cwd.
	if err := unmount(".", unix.MNT_DETACH); err != nil {
		return err
	}

	// Switch back to our shiny new root.
	if err := unix.Chdir("/"); err != nil {
		return &os.PathError{Op: "chdir", Path: "/", Err: err}
	}
	return nil
}

// The Linux kernel automatically removes a namespace whenever the last process that’s part of it terminates. 
// There is a technique however to keep a namespace around by bind mounting it, even if no processes are part of it.
func setupNewMountNamespace(processID int) {
	// open the processes’s network namespace file, which is in /proc/self/ns/net.
	// This is to save the fd reference of the current namespace before we unshare (so we can set it back)
  fd, err := syscall.Open("/proc/self/ns/net", syscall.O_RDONLY, 0)
  defer syscall.Close(fd)
  if err != nil {
    log.Fatalf("Unable to open: %v\n", err)
  }

	// This disassociates the current process with the namespace it is part of, 
	// creates a fresh new network namespace and sets it as the network namespace for the process. 
  if err := syscall.Unshare(syscall.CLONE_NEWNET); err != nil {
    log.Fatalf("Unshare system call failed: %v\n", err)
  }

	// bind mount the (new) network namespace special file of this process to a known file name, which is /var/run//net-ns/<container-id>
	// Such that this file can then anytime be used to refer to this network namespace.
	// But also since in the next step with remove this process from this namespace, we want to retain (so that's why it is bind-mounted to nsMount)
  if err := syscall.Mount("/proc/self/ns/net", nsMount, 
                                "bind", syscall.MS_BIND, ""); err != nil {
    log.Fatalf("Mount system call failed: %v\n", err)
  }

	// sets the namespace of the current process back to the one specified by the file descriptor obtained earlier.
	// syscall.CLONE_NEWNET flag indicates that it's setting the network namespace.
  if err := unix.Setns(fd, syscall.CLONE_NEWNET); err != nil {
    log.Fatalf("Setns system call failed: %v\n", err)
  }
}

func main() {
	out, err := exec.Command("readlink", "/proc/self/ns/mnt").Output(); if err != nil {
		log.Fatalf("Error reading namespace file: %v\n", err)
	}
	log.Printf("Process is now in the current Namespace: %s", string(out))
}