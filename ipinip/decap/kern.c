//go:build ignore

#include <stdbool.h>
#include <string.h>
#include <linux/stddef.h>
#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/in.h>
#include <linux/ip.h>
#include <linux/ipv6.h>
#include <linux/mpls.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/pkt_cls.h>
#include <linux/types.h>

#include <bpf/bpf_endian.h>
#include <bpf/bpf_helpers.h>

static int decap_internal(struct __sk_buff *skb, int off, int len, char proto)
{
	switch (proto) {
	case IPPROTO_IPIP:
		break;
	default:
		return TC_ACT_OK;
	}

  int olen = len;
  if (bpf_skb_adjust_room(skb, -olen, BPF_ADJ_ROOM_MAC, BPF_F_ADJ_ROOM_FIXED_GSO))
    return TC_ACT_SHOT;

	return TC_ACT_OK;
}

static int decap_ipv4(struct __sk_buff *skb)
{
	struct iphdr iph_outer;

	if (bpf_skb_load_bytes(skb, ETH_HLEN, &iph_outer,
			       sizeof(iph_outer)) < 0)
		return TC_ACT_OK;

	if (iph_outer.ihl != 5)
		return TC_ACT_OK;

	return decap_internal(skb, ETH_HLEN, sizeof(iph_outer),
			      iph_outer.protocol);
}

SEC("tc")
int tc_decap(struct __sk_buff *skb) {
  if (skb->protocol == bpf_htons(ETH_P_IP)) {
    return decap_ipv4(skb);
  }
  else {
    return TC_ACT_OK;
  }
}

char __license[] SEC("license") = "GPL";