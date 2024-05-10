package main

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go sockmap sockmap.c

import (
	"bufio"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
)

func main() {
	// Remove resource limits for kernels <5.11.
	if err := rlimit.RemoveMemlock(); err != nil { 
			log.Fatal("Removing memlock:", err)
	}

	// Load the compiled eBPF ELF and load it into the kernel.
	var objs sockmapObjects 
	if err := loadSockmapObjects(&objs, nil); err != nil {
			log.Fatal("Loading eBPF objects:", err)
	}
	defer objs.Close() 

	// Get the first-mounted cgroupv2 path.
	cgroupPath, err := detectCgroupPath()
	if err != nil {
		log.Fatal(err)
	}

	// Link the count_egress_packets program to the cgroup.
	sockopsLink, err := link.AttachCgroup(link.CgroupOptions{
		Path:    cgroupPath,
		Attach:  ebpf.AttachCGroupSockOps,
		Program: objs.SockopsProg,
	})
	if err != nil {
			log.Fatal("Attaching SockMap to Cgroup:", err)
	}
	defer sockopsLink.Close() 

	// Link the count_egress_packets program to the cgroup.
	if err := link.RawAttachProgram(link.RawAttachProgramOptions{
		Target:  objs.SockMap.FD(), // TODO: what is this?
		Attach:  ebpf.AttachSkMsgVerdict,
		Program: objs.SkMsgProg,
	}); err != nil {
		log.Fatal("Attaching SkMsg to Cgroup:", err)
	}
	//defer skmsgLink.Close() 	

	for {
		time.Sleep(time.Second * 2)
	}
}

// detectCgroupPath returns the first-found mount point of type cgroup2
// and stores it in the cgroupPath global variable.
func detectCgroupPath() (string, error) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		// example fields: cgroup2 /sys/fs/cgroup/unified cgroup2 rw,nosuid,nodev,noexec,relatime 0 0
		fields := strings.Split(scanner.Text(), " ")
		if len(fields) >= 3 && fields[2] == "cgroup2" {
			return fields[1], nil
		}
	}

	return "", errors.New("cgroup2 not mounted")
}
