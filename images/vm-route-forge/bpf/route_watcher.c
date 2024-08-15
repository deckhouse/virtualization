#include "lib/types.h"
#include "lib/ip_fib_less.h"
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

struct {
    __uint(type, BPF_MAP_TYPE_QUEUE);
    __uint(key_size, 0);
    __uint(value_size, sizeof(struct route_event));
    __uint(max_entries, 1 << 10);
    __uint(map_flags, 0);
} route_events_map SEC(".maps");

static inline int insert_event(struct pt_regs *ctx, u32 action) {
    struct route_event evt = {
        .action = action,
    };
    struct fib_table *tb;
    struct fib_config *cfg;
    int ret;

    ret = bpf_probe_read(&tb, sizeof(tb), (void *)&PT_REGS_PARM2(ctx));
    if (!tb) {
        static const char msg[] = "Failed to read fib_table pointer: %d";
        bpf_trace_printk(msg, sizeof(msg), ret);
        return ret;
    }

    ret = bpf_probe_read(&evt.table, sizeof(evt.table), &tb->tb_id);
    if (ret < 0) {
        static const char msg[] = "Failed to read tb_id: %d";
        bpf_trace_printk(msg, sizeof(msg), ret);
        return ret;
    }

    ret = bpf_probe_read(&cfg, sizeof(cfg), (void *)&PT_REGS_PARM3(ctx));
    if (!cfg) {
        static const char msg[] = "Failed to read fib_config pointer: %d";
        bpf_trace_printk(msg, sizeof(msg), ret);
        return ret;
    }

    ret = bpf_probe_read(&evt.dst, sizeof(evt.dst), &cfg->fc_dst);
    if (ret < 0) {
        static const char msg[] = "Failed to read dst: %d";
        bpf_trace_printk(msg, sizeof(msg), ret);
        return ret;
    }

    ret = bpf_probe_read(&evt.src, sizeof(evt.src), &cfg->fc_prefsrc);
    if (ret < 0) {
        static const char msg[] = "Failed to read src: %d";
        bpf_trace_printk(msg, sizeof(msg), ret);
        return ret;
    }

    return bpf_map_push_elem(&route_events_map, &evt, BPF_ANY);
}

SEC("kprobe/fib_table_insert")
int kprobe__fib_table_insert(struct pt_regs *ctx) {
    insert_event(ctx, 0);
    return 0;
}

SEC("kprobe/fib_table_delete")
int kprobe__fib_table_delete(struct pt_regs *ctx) {
    insert_event(ctx, 1);
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
