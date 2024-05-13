package main

import (
	"os"
	"os/exec"
	"log"
	"syscall"
)

func setupNewMountNamespace(newRoot string, putOld string) {
	// bind mount newroot to itself - this is a slight hack
	// PIVOT_ROOT REQUIREMENT - "new_root must be a path to a mount point"
	// MS_BIND - create a bind mount
	// MS_REC - Apply recursively mount (action) to all submounts of the source
	if err := syscall.Mount(newRoot, newRoot, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		log.Fatalln("failed to mount new root filesystem: ", err)
	}

	// create directory for old root 
	// PIVOT_ROOT REQUIREMENT - put_old must be at or underneath new_root
	if err := syscall.Mkdir(newRoot+putOld, 0700); err != nil {
		log.Fatalln("failed to mkdir: ", err)
	}

	// This disassociates the current process with the mount namespace it is part of, 
	// creates a fresh new mount namespace
  if err := syscall.Unshare(syscall.CLONE_NEWNS); err != nil {
    log.Fatalf("Unshare system call failed: %v\n", err)
  }

	// pivot_root to the new root filesystem
	// NOTE: If this doesn't work, recheck the requirements in the link above
	if err := syscall.PivotRoot(newRoot, newRoot+putOld); err != nil {
		log.Fatalln("failed to pivot root: ", err)
	}

	// change the current working directory to "/"" in the new mount namespace
	if err := syscall.Chdir("/"); err != nil {
		log.Fatalln("failed to chdir to /: ", err)
	}

	// mount /proc
	if err := syscall.Mount("/proc", "/proc", "proc", 0, ""); err != nil {
		log.Fatalln("failed to mount /proc: ", err)
	}

	// Requires to be able to call commands like `mount` and 'readlink' below
	if err := syscall.Mount("/dev", "/dev", "tmpfs", 0, ""); err != nil {
		log.Fatalln("failed to mount /dev: ", err)
	}
	file, err := os.Create("/dev/null"); if err != nil {
			log.Fatal(err)
	}
	defer file.Close()

	// unmount the old root filesystem
	if err := syscall.Unmount(putOld, syscall.MNT_DETACH); err != nil {
		log.Fatalln("failed to unmount the old root filesystem: ", err)
	}
}

func main() {
	processID := os.Getpid()
	log.Printf("Process ID: %d\n", processID)

	// Check the current mount namespace
	out, err := exec.Command("readlink", "/proc/self/ns/mnt").Output(); if err != nil {
		log.Fatalf("Error reading namespace file: %v\n", err)
	}
	log.Printf("Process is now in the old mount Namespace: %s", string(out))

	newRoot := "new_root"
	putOld := "/old_root"
	setupNewMountNamespace(newRoot, putOld)

	// Check the current mount namespace
	out1, err := exec.Command("readlink", "/proc/self/ns/mnt").Output(); if err != nil {
		log.Fatalf("Error reading namespace file: %v\n", err)
	}
	log.Printf("Process is now in the new mount Namespace: %s", string(out1))

	log.Println("Opening a shell (bin/sh) in the new mount namespace - run commands like `mount`, 'lsns', etc.")
	cmd := exec.Command("/bin/sh")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Println("failed to run the command: ", err)
		os.Exit(1)
	}
}