package main

import (
	"log"
	"os"
	"os/exec"
	"syscall"
	"fmt"
	"strconv"

	"golang.org/x/sys/unix"
)

func getGockerNetNsPath() string {
	return "/var/run/gocker/net-ns"
}

func createDirsIfDontExist(dirs []string) error {
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err = os.MkdirAll(dir, 0755); err != nil {
				log.Printf("Error creating directory: %v\n", err)
				return err
			}
		}
	}
	return nil
}

// The Linux kernel automatically removes a namespace whenever the last process that’s part of it terminates. 
// There is a technique however to keep a namespace around by bind mounting it, even if no processes are part of it.
func setupNewNetworkNamespace(processID int) {
  _ = createDirsIfDontExist([]string{getGockerNetNsPath()})
  nsMount := getGockerNetNsPath() + "/" + strconv.Itoa(processID)
  if _, err := syscall.Open(nsMount, 
                syscall.O_RDONLY|syscall.O_CREAT|syscall.O_EXCL,
                0644); err != nil {
    log.Fatalf("Unable to open bind mount file: :%v\n", err)
  }
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

	// bind mount the (new) network namespace special file of this process to a known file name, which is /var/run/gocker/net-ns/<container-id>
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

func joinContainerNetworkNamespace(processID int) error {
	nsMount := getGockerNetNsPath() + "/" + strconv.Itoa(processID)
	fd, err := unix.Open(nsMount, unix.O_RDONLY, 0)
	if err != nil {
		log.Printf("Unable to open: %v\n", err)
		return err
	}
	if err := unix.Setns(fd, unix.CLONE_NEWNET); err != nil {
		log.Printf("Setns system call failed: %v\n", err)
		return err
	}
	return nil
}

func main () {
	processID := os.Getpid()
	log.Printf("Process ID: %d\n", processID)
	path := fmt.Sprintf("/proc/%d/ns/net", processID)
	out, err := exec.Command("readlink", path).Output()
	if err != nil {
		log.Fatalf("Error reading namespace file: %v\n", err)
	}
	log.Printf("ID of the current Namespace is: %s", string(out))

	setupNewNetworkNamespace(processID)
	joinContainerNetworkNamespace(processID)
	out2, err := exec.Command("readlink", path).Output()
	if err != nil {
		log.Fatalf("Error reading namespace file: %v\n", err)
	}
	log.Printf("ID of the new Namespace is: %s", string(out2))
}