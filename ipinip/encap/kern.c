//go:build ignore

// NOTE: IPIP encapsulation goes as follows:
// 1. Adjust the head of the packet to make room for the new header
// 2. Recalculate the pointers to the packet data
// 3. Set the values of "new" (actually just shifted) ethhdr to the old ethhdr values
// 4. Set values of the IPIP header (old IP stays where it is
// 		- we just increased the packet size, 
// 		- shifted ethhdr to make the space shift in between the new ethhdr and the old iphdr to include in between the IPIP header)
// 5. Recalculate the checksum
// 6. Count the packet as transmitted

#include <linux/bpf.h>
#include <linux/in.h>
#include <linux/if_ether.h>
#include <linux/if_packet.h>
#include <linux/if_vlan.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/socket.h>
#include <linux/ipv6.h>
#include <linux/types.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>
#include "parse_helpers.h"

/* Supported address families. */
#define AF_UNSPEC	0
#define AF_UNIX		1	/* Unix domain sockets 		*/
#define AF_INET		2	/* Internet IP Protocol 	*/
#define AF_AX25		3	/* Amateur Radio AX.25 		*/
#define AF_IPX		4	/* Novell IPX 			*/
#define AF_APPLETALK	5	/* Appletalk DDP 		*/
#define	AF_NETROM	6	/* Amateur radio NetROM 	*/
#define AF_BRIDGE	7	/* Multiprotocol bridge 	*/
#define AF_AAL5		8	/* Reserved for Werner's ATM 	*/
#define AF_X25		9	/* Reserved for X.25 project 	*/
#define AF_INET6	10	/* IP version 6			*/
#define AF_MAX		12	/* For now.. */
#define MAX_IPTNL_ENTRIES 256U

struct vip {
	__u32 daddr; // IPv4 only
	__u16 dport;
	__u16 family;
	__u8 protocol;
};
struct iptnl_info {
	__u32 saddr; // IPv4 only
	__u32 daddr; // IPv4 only
	__u16 family;
	__u8 dmac[6];
};

// Force emitting structs into the ELF.
const struct vip *unused __attribute__((unused));
const struct iptnl_info *unused2 __attribute__((unused));

// This map is used to store the tunnel information (IPIP header values) for each VIP
struct bpf_map_def SEC("maps") vip2tnl = {
	.type = BPF_MAP_TYPE_HASH,
	.key_size = sizeof(struct vip),
	.value_size = sizeof(struct iptnl_info),
	.max_entries = MAX_IPTNL_ENTRIES,
};

static __always_inline int handle_ipv4(struct xdp_md *xdp)
{
	void *data_end = (void *)(long)xdp->data_end;
	void *data = (void *)(long)xdp->data;
	struct ethhdr *eth;
	struct iphdr *ip = data + sizeof(struct ethhdr); 
	struct hdr_cursor nh;
	nh.pos = data;

	int eth_type = parse_ethhdr(&nh, data_end, &eth);
	int ip_type = parse_iphdr(&nh, data_end, &ip);
	if ((eth_type != bpf_htons(ETH_P_IP)) || (ip_type != IPPROTO_TCP))
		return XDP_PASS;
	if ((void *)(ip + 1) > data_end)
		return XDP_DROP;

	struct tcphdr *tcp;
	int tcp_type = parse_tcphdr(&nh, data_end, &tcp);
	if ((void*)(tcp + 1) > data_end) {
		return XDP_PASS;
	}
	
	// Retrieve packet metadata to match it to VIP
	struct vip vip = {};
	vip.protocol = ip->protocol;
	vip.family = AF_INET;
	vip.daddr = bpf_ntohl(ip->daddr);
	vip.dport = bpf_ntohs(tcp->dest);
	__u16 payload_len = bpf_ntohs(ip->tot_len);

	// For the VIP retrieve the encapsulation information
	struct iptnl_info *tnl;
	tnl = bpf_map_lookup_elem(&vip2tnl, &vip);
	
	// For now we only do IPv4 in IPv4 encapsulation
	if (!tnl || tnl->family != AF_INET) {
		return XDP_PASS;
	}

	/* The vip key is found.  Add an IP header and send it out */
	if (bpf_xdp_adjust_head(xdp, 0 - (int)sizeof(struct iphdr)))
		return XDP_DROP;

	// Reverify packet pointers, because bpf_xdp_adjust_head resets it
	data = (void *)(long)xdp->data;
	data_end = (void *)(long)xdp->data_end;

	struct ethhdr *new_eth;
	struct ethhdr *old_eth;
	new_eth = data; // This part of the packet is actually empty, because we just adjusted the head
	ip = data + sizeof(*new_eth); // iphdr is offset by the size of the ethhdr struct as is always
	old_eth = data + sizeof(*ip); // Old ethhdr is offset by the size of the ip struct
	
	if ((void *)(new_eth + 1) > data_end ||
	    (void *)(old_eth + 1) > data_end ||
	    (void *)(ip + 1) > data_end)
		return XDP_DROP;

	// Retain the old ethhdr values into the new ethhdr
	__builtin_memcpy(new_eth->h_source, old_eth->h_dest, sizeof(new_eth->h_dest));
	__builtin_memcpy(new_eth->h_dest, tnl->dmac, sizeof(new_eth->h_dest));
	new_eth->h_proto = bpf_htons(ETH_P_IP);

	bpf_printk("tnl->dmac: %x", tnl->dmac);
	bpf_printk("Old eth dest: %x", old_eth->h_dest);
	bpf_printk("New eth dest: %x", new_eth->h_dest);
	bpf_printk("Eth src: %x", new_eth->h_source);

	// Set the IPIP header (new header wraps the old iphdr)
	ip->version = 4;
	ip->ihl = sizeof(*ip) >> 2;
	ip->frag_off =	0;
	ip->protocol = IPPROTO_IPIP;
	ip->check = 0;
	ip->tos = 0;
	ip->tot_len = bpf_htons(payload_len + sizeof(*ip));
	ip->daddr = bpf_htonl(ip->daddr);
	ip->saddr = bpf_htonl(tnl->saddr);
	ip->ttl = 8;
	__u16 *next_iph___u16;
	next_iph___u16 = (__u16 *)ip;
	
	// Recalculate the checksum
	int i;
	__u32 csum = 0;
	#pragma clang loop unroll(full)
	for (i = 0; i < sizeof(*ip) >> 1; i++)
		csum += *next_iph___u16++;
	ip->check = ~((csum & 0xffff) + (csum >> 16));

	bpf_printk("Sending encapsulated packet\n");

	return XDP_TX;
}

SEC("xdp")
int xdp_tx_iptunnel(struct xdp_md *xdp)
{
	void *data_end = (void *)(long)xdp->data_end;
	void *data = (void *)(long)xdp->data;
	struct ethhdr *eth = data;
	__u16 h_proto;

	if ((void *)(eth + 1) > data_end)
		return XDP_DROP;

	h_proto = eth->h_proto;
	if (h_proto == bpf_htons(ETH_P_IP))
		return handle_ipv4(xdp);
	else {
		return XDP_PASS;
	}
}

char _license[] SEC("license") = "GPL";