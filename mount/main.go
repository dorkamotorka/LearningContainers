package main

import (
	"os"
	"os/exec"
	"log"
	"syscall"
	//"strconv"
	//"golang.org/x/sys/unix"
)


/*
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
*/

// The Linux kernel automatically removes a namespace whenever the last process that’s part of it terminates. 
// There is a technique however to keep a namespace around by bind mounting it, even if no processes are part of it.
func setupNewMountNamespace() {
	newRoot := "rootfs3"
	putOld := "/old_root" // Will and MUST be inside rootfs!
	// 1. mount alpine root file system as a mountpoint, then it can be used to pivot_root
	if err := syscall.Mount(newRoot, newRoot, "", syscall.MS_BIND, ""); err != nil {
		log.Println("failed to mount new root filesystem: ", err)
		os.Exit(1)
	}

	if err := syscall.Mkdir(newRoot+putOld, 0700); err != nil {
		log.Println("failed to mkdir: ", err)
		os.Exit(1)
	}
	go os.RemoveAll(putOld)

	// This disassociates the current process with the namespace it is part of, 
	// creates a fresh new mount namespace and sets it as the mount namespace for the process. 
  if err := syscall.Unshare(syscall.CLONE_NEWNS); err != nil {
    log.Fatalf("Unshare system call failed: %v\n", err)
  }

	/*
	cmd := exec.Command("/bin/bash")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Println("failed to run the command: ", err)
		os.Exit(1)
	}
	*/

	if err := syscall.PivotRoot(newRoot, newRoot+putOld); err != nil {
		log.Println("failed to pivot root: ", err)
		os.Exit(1)
	}

	if err := syscall.Chdir("/"); err != nil {
		log.Println("failed to chdir to /: ", err)
		os.Exit(1)
	}

	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		log.Println("failed to mount /proc: ", err)
		os.Exit(1)
	}

	// unmount the old root filesystem
	if err := syscall.Unmount(putOld, syscall.MNT_DETACH); err != nil {
		log.Println("failed to unmount the old root filesystem: ", err)
		os.Exit(1)
	}
}

func main() {
	processID := os.Getpid()
	log.Printf("Process ID: %d\n", processID)

	out, err := exec.Command("readlink", "/proc/self/ns/mnt").Output(); if err != nil {
		log.Fatalf("Error reading namespace file: %v\n", err)
	}
	log.Printf("Process is now in the current Namespace: %s", string(out))

	setupNewMountNamespace()

	out1, err := exec.Command("readlink", "/proc/self/ns/mnt").Output(); if err != nil {
		log.Fatalf("Error reading namespace file: %v\n", err)
	}
	log.Printf("Process is now in the current Namespace: %s", string(out1))
}