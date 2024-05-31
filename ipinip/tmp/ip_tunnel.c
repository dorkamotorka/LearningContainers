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

static const int cfg_port = 8000;

#define	L2_PAD_SZ	(sizeof(struct vxlanhdr) + ETH_HLEN)

#define BPF_F_ADJ_ROOM_DECAP_L3_IPV4 0x80

struct vxlanhdr {
	__be32 vx_flags;
	__be32 vx_vni;
} __attribute__((packed));

struct gre_hdr {
	__be16 flags;
	__be16 protocol;
} __attribute__((packed));

union l4hdr {
	struct udphdr udp;
	struct gre_hdr gre;
};

struct v4hdr {
	struct iphdr ip;
	union l4hdr l4hdr;
	__u8 pad[L2_PAD_SZ];		/* space for L2 header / vxlan header ... */
} __attribute__((packed));

static __always_inline void set_ipv4_csum(struct iphdr *iph)
{
	__u16 *iph16 = (__u16 *)iph;
	__u32 csum;
	int i;

	iph->check = 0;

	#pragma unroll
	for (i = 0, csum = 0; i < sizeof(*iph) >> 1; i++)
		csum += *iph16++;

	iph->check = ~((csum & 0xffff) + (csum >> 16));
}

static __always_inline int __encap_ipv4(struct __sk_buff *skb, __u8 encap_proto,
					__u16 l2_proto, __u16 ext_proto)
{
	struct iphdr iph_inner;
	struct v4hdr h_outer;
	struct tcphdr tcph;
	int olen, l2_len;
	__u8 *l2_hdr = NULL;
	int tcp_off;
	__u64 flags;

  if (bpf_skb_load_bytes(skb, ETH_HLEN, &iph_inner, sizeof(iph_inner)) < 0)
    return TC_ACT_OK;

  tcp_off = sizeof(iph_inner);

	/* filter only packets we want */
	if (iph_inner.ihl != 5 || iph_inner.protocol != IPPROTO_TCP)
		return TC_ACT_OK;

	if (bpf_skb_load_bytes(skb, ETH_HLEN + tcp_off, &tcph, sizeof(tcph)) < 0)
		return TC_ACT_OK;

	if (tcph.dest != __bpf_constant_htons(cfg_port))
		return TC_ACT_OK;

	olen = sizeof(h_outer.ip);
	l2_len = 0;

	flags = BPF_F_ADJ_ROOM_FIXED_GSO | BPF_F_ADJ_ROOM_ENCAP_L3_IPV4;
	flags |= BPF_F_ADJ_ROOM_ENCAP_L2(l2_len);

	switch (encap_proto) {
	case IPPROTO_IPIP:
  case IPPROTO_IPV6:
		break;
	default:
		return TC_ACT_OK;
	}

	l2_hdr = (__u8 *)&h_outer + olen;
	olen += l2_len;

	/* add room between mac and network header */
	if (bpf_skb_adjust_room(skb, olen, BPF_ADJ_ROOM_MAC, flags))
		return TC_ACT_SHOT;

	/* prepare new outer network header */
	h_outer.ip = iph_inner;
	h_outer.ip.tot_len = bpf_htons(olen + bpf_ntohs(h_outer.ip.tot_len));
	h_outer.ip.protocol = encap_proto;

	set_ipv4_csum((void *)&h_outer.ip);

	/* store new outer network header */
	if (bpf_skb_store_bytes(skb, ETH_HLEN, &h_outer, olen, BPF_F_INVALIDATE_HASH) < 0)
		return TC_ACT_SHOT;

	return TC_ACT_OK;
}

static __always_inline int encap_ipv4(struct __sk_buff *skb, __u8 encap_proto,
				      __u16 l2_proto)
{
	return __encap_ipv4(skb, encap_proto, l2_proto, 0);
}

SEC("encap_ipip_none")
int __encap_ipip_none(struct __sk_buff *skb)
{
	if (skb->protocol == __bpf_constant_htons(ETH_P_IP))
		return encap_ipv4(skb, IPPROTO_IPIP, ETH_P_IP);
	else
		return TC_ACT_OK;
}

static int decap_internal(struct __sk_buff *skb, int off, int len, char proto)
{
	__u64 flags = BPF_F_ADJ_ROOM_FIXED_GSO;
	int olen = len;

	switch (proto) {
	case IPPROTO_IPIP:
		flags |= BPF_F_ADJ_ROOM_DECAP_L3_IPV4;
		break;
	default:
		return TC_ACT_OK;
	}

	if (bpf_skb_adjust_room(skb, -olen, BPF_ADJ_ROOM_MAC, flags))
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

SEC("decap")
int decap_f(struct __sk_buff *skb)
{
	switch (skb->protocol) {
	case __bpf_constant_htons(ETH_P_IP):
		return decap_ipv4(skb);
	case __bpf_constant_htons(ETH_P_IPV6):
		//return decap_ipv6(skb);
	default:
		return TC_ACT_OK;
	}
}

char __license[] SEC("license") = "GPL";