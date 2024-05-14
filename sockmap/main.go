package main

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go sockmap sockmap.c

import (
	"log"
	"time"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
)

func main() {
	// Remove resource limits for kernels <5.11.
	if err := rlimit.RemoveMemlock(); err != nil { 
			log.Print("Removing memlock:", err)
	}

	// Load the compiled eBPF ELF and load it into the kernel.
	var objs sockmapObjects 
	if err := loadSockmapObjects(&objs, nil); err != nil {
			log.Print("Loading eBPF objects:", err)
	}
	defer objs.Close() 

	sockopsLink, err := link.AttachCgroup(link.CgroupOptions{
		Path:    "/sys/fs/cgroup/unified/test.slice", // Path to the cgroup v2 hierarchy
		Attach:  ebpf.AttachCGroupSockOps,
		Program: objs.SockopsProg,
	})
	if err != nil {
			log.Print("Attaching SockMap to Cgroup:", err)
	}
	defer sockopsLink.Close() 

	if err := link.RawAttachProgram(link.RawAttachProgramOptions{
		Target:  objs.SockMap.FD(), // Attach to a BPF map you want to run the eBPF program on when events occur
		Attach:  ebpf.AttachSkMsgVerdict,
		Program: objs.SkMsgProg,
	}); err != nil {
		log.Print("Attaching SkMsg to Cgroup:", err)
	}

	log.Println("Prepared BPF programs...")

	for {
		time.Sleep(time.Second * 2)
	}
}
