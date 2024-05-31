package main

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go kern kern.c

import (
	"log"
	"net"
	"flag"
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

	var ifname string
	flag.StringVar(&ifname, "i", "lo", "Network interface name where the eBPF program will be attached")
	flag.Parse()

	// Load the compiled eBPF ELF and load it into the kernel.
	var objs kernObjects 
	if err := loadKernObjects(&objs, nil); err != nil {
			log.Fatal("Loading eBPF objects:", err)
	}
	defer objs.Close() 

	iface, err := net.InterfaceByName(ifname)
	if err != nil {
			log.Fatalf("Getting interface %s: %s", ifname, err)
	}

	// Attach count_packets to the network interface.
	tclink, err := link.AttachTCX(link.TCXOptions{ 
		Program:   objs.TcDecap,
		Attach:		 ebpf.AttachTCXEgress,
		Interface: iface.Index,
	})
	if err != nil {
			log.Fatal("Attaching TC:", err)
	}
	defer tclink.Close() 

	log.Printf("Doing port remapping on %s..", ifname)

	for {
		time.Sleep(1 * time.Second)
	}
}