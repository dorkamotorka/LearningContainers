package main

// "-type" => name of a type to generate a Go declaration for, may be repeated
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -type vip -type iptnl_info kern kern.c

import (
	"flag"
	"log"
	"net"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"golang.org/x/sys/unix"
)

func main() {
	// Remove resource limits for kernels <5.11.
	if err := rlimit.RemoveMemlock(); err != nil { 
		log.Fatal("Removing memlock:", err)
	}

	var ifname string
	flag.StringVar(&ifname, "i", "enp5s0", "Network interface name where the eBPF program will be attached")
	flag.Parse()

	// Load the compiled eBPF ELF and load it into the kernel.
	var objs kernObjects 
	if err := loadKernObjects(&objs, nil); err != nil {
			log.Fatal("Loading eBPF objects:", err)
	}
	defer objs.Close() 

	iface, err := net.InterfaceByName(ifname)
	if err != nil {
			log.Fatalf("Failed to get interface %s: %s", ifname, err)
	}

	// Attach XDP program to the network interface.
	xdplink, err := link.AttachXDP(link.XDPOptions{ 
			Program:   objs.XdpTxIptunnel,
			Interface: iface.Index,
	})
	if err != nil {
			log.Fatal("Attaching XDP:", err)
	}
	defer xdplink.Close()

	var tnl kernIptnlInfo
	tnl.Family = unix.AF_INET
	tnl.Saddr = uint32(1489508137) // 88.200.23.41 (s1)
	tnl.Daddr = uint32(1489508136) // 88.200.23.40 (s2)
	tnl.Dmac = [6]byte{0xdd, 0xee, 0x20, 0x3e, 0x16, 0x00 } // "htonl" 00:16:3e:20:ee:dd MAC address

	var vip kernVip
	vip.Protocol = unix.IPPROTO_TCP
	vip.Family = unix.AF_INET
	vip.Dport = 8081
	vip.Daddr = uint32(1489508238) // 88.200.23.142 (tjp.lrk.si)

	// Update the eBPF map with the tunnel and VIP information.
	if err := objs.Vip2tnl.Update(&vip, &tnl, ebpf.UpdateAny); err != nil {
		log.Fatal("Failed to update Vip2tnl map:", err)
	}

	for {
		time.Sleep(1 * time.Second)
	}
}