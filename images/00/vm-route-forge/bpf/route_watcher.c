#include "include/types.h"
#include "include/ip_fib_less.h"
#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <linux/ptrace.h>

struct route_event {
    u32 action; // 0 - add, 1 - delete
    u32 table;
    __be32 dst;
    __be32 src;
};

// Force emitting struct event into the ELF.
const struct route_event *unused __attribute__((unused));

// Define a map with the BPF_MAP_TYPE_QUEUE type.
// BPF_MAP_TYPE_QUEUE provides FIFO storage.
struct {
    __uint(type, BPF_MAP_TYPE_QUEUE);
    __uint(key_size, 0);
    __uint(value_size, sizeof(struct route_event));
    __uint(max_entries, 1 << 16);
    __uint(map_flags, 0);
} route_events_map SEC(".maps");

static inline int insert_event(struct pt_regs *ctx, u32 action) {
    struct route_event evt = {
        .action = action,
    };
    struct fib_table *tb;
    struct fib_config *cfg;
    int ret;

    // Save the fib_table from the ctx to the tb.
    ret = bpf_probe_read_kernel(&tb, sizeof(tb), (void *)&PT_REGS_PARM2(ctx));
    if (!tb) {
        static const char msg[] = "Failed to read fib_table pointer: %d";
        bpf_trace_printk(msg, sizeof(msg), ret);
        return ret;
    }

    // Save the table id from the tb to the evt.
    ret = bpf_probe_read_kernel(&evt.table, sizeof(evt.table), &tb->tb_id);
    if (ret < 0) {
        static const char msg[] = "Failed to read tb_id: %d";
        bpf_trace_printk(msg, sizeof(msg), ret);
        return ret;
    }

    // Save the fib_config from the ctx to the cfg.
    ret = bpf_probe_read_kernel(&cfg, sizeof(cfg), (void *)&PT_REGS_PARM3(ctx));
    if (!cfg) {
        static const char msg[] = "Failed to read fib_config pointer: %d";
        bpf_trace_printk(msg, sizeof(msg), ret);
        return ret;
    }

    // Save the dst address from the cfg to the evt.
    ret = bpf_probe_read_kernel(&evt.dst, sizeof(evt.dst), &cfg->fc_dst);
    if (ret < 0) {
        static const char msg[] = "Failed to read dst: %d";
        bpf_trace_printk(msg, sizeof(msg), ret);
        return ret;
    }

    // Save the src address from the cfg to the evt.
    ret = bpf_probe_read_kernel(&evt.src, sizeof(evt.src), &cfg->fc_prefsrc);
    if (ret < 0) {
        static const char msg[] = "Failed to read src: %d";
        bpf_trace_printk(msg, sizeof(msg), ret);
        return ret;
    }

    // Add route_event to the end of the bpf map.
    // We never delete objects from the map from the bpf program, because our program does this in the user namespace.
    return bpf_map_push_elem(&route_events_map, &evt, BPF_ANY);
}

SEC("kprobe/fib_table_insert")
// int fib_table_insert(struct net *, struct fib_table *, struct fib_config *,
//   		     struct netlink_ext_ack *extack);
// The parameters of fib_table_insert are saved in ctx
int fib_table_insert(struct pt_regs *ctx) {
    insert_event(ctx, 0);
    return 0;
}

SEC("kprobe/fib_table_delete")
// int fib_table_delete(struct net *, struct fib_table *, struct fib_config *,
//   		     struct netlink_ext_ack *extack);
// The parameters of fib_table_delete are saved in ctx
int fib_table_delete(struct pt_regs *ctx) {
    insert_event(ctx, 1);
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
