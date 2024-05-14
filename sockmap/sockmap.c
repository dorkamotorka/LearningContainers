//go:build ignore

#include <linux/bpf.h>
#include <asm/socket.h>
#include <bpf/bpf_endian.h>
#include <bpf/bpf_helpers.h>

/* Map local port in host byte order to target socket */
// BPF map that holds weak references to socket (weak, because it is not responsible to keep them alive)
// Two BPF Map types:
//   - BPF_MAP_TYPE_SOCKMAP (<32-bit integer>: socket)
//   - BPF_MAP_TYPE_SOCKHASH (<arbitrary-data e.g. string, struct, etc.>: socket)
struct {
	__uint(type, BPF_MAP_TYPE_SOCKHASH);
	__uint(max_entries, 2);
	__type(key, __u32);
	__type(value, __u64);
} sock_map SEC(".maps");

char _license[] SEC("license") = "GPL";


// Two types of programs:
//  - BPF_PROG_TYPE_SK_MSG: attached to BPF_MAP_TYPE_SOCKMAP or BPF_MAP_TYPE_SOCKHASH maps and will be invoked sendmsg or sendfile syscalls are executed on sockets which are part of the map the program is attached to.
//  - BPF_PROG_TYPE_SK_SKB: attached to BPF_MAP_TYPE_SOCKMAP or BPF_MAP_TYPE_SOCKHASH maps and will be invoked when messages get received on the sockets which are part of the map the program is attached to
SEC("sk_msg")
int sk_msg_prog(struct sk_msg_md *msg)
{
	__u32 lport = msg->local_port;

	// Redirect the message to the socket associated with the local port
	// BPF_F_INGRESS: Redirect the message to the socket associated with the local port (ingress traffic)
	// BPF_F_EGRESS: Redirect the message to the socket associated with the remote port (egress traffic)
	// returns SK_PASS on success, or SK_DROP on error
	return bpf_msg_redirect_hash(msg, &sock_map, &lport, BPF_F_INGRESS);
}

// This program is called whenever there's a socket operation on a particular cgroup (retransmit timeout, connection establishment, etc.)
// For this example on such events we add the sockets to our socket map if they are passive (server-side socket)
SEC("sockops")
int sockops_prog(struct bpf_sock_ops *ctx)
{
	__u32 lport = bpf_ntohl(ctx->remote_port);

	// op is the operation that is being performed on the socket
	switch (ctx->op) {
		// An active socket is connecting to a remote active socket using connect()
		case BPF_SOCK_OPS_ACTIVE_ESTABLISHED_CB:
		// A passive socket is not connected, but rather awaits an incoming connection,
		// which will spawn a new active socket once a connection is established using listen() and accept()
		case BPF_SOCK_OPS_PASSIVE_ESTABLISHED_CB:
			// socket cookie can be thought of as a global socket identifier that can be assumed unique,
			// which can be useful for monitoring per socket networking traffic statistics
			bpf_printk("Add entry to SOCKHASH map => %u : %lx (local_port : sk)\n", lport, bpf_get_socket_cookie(ctx));
			// Update the socket map with the local port and the socket context
			// Add an entry to, or update a sockhash map referencing sockets => (lport: ctx), where ctx is bpf_sock_ops struct
			bpf_sock_hash_update(ctx, &sock_map, &lport, BPF_ANY);
			break;
	}

	return SK_PASS;
}

