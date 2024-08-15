#include "types.h"

typedef u8 dscp_t;

struct in6_addr {
	union {
		__u8 u6_addr8[16];
		__be16 u6_addr16[8];
		__be32 u6_addr32[4];
	} in6_u;
};

struct nl_info {
	struct nlmsghdr *nlh;
	struct net *nl_net;
	u32 portid;
	u8 skip_notify: 1;
	u8 skip_notify_kernel: 1;
};

struct hlist_node;

struct hlist_node {
	struct hlist_node *next;
	struct hlist_node **pprev;
};

struct callback_head {
	struct callback_head *next;
	void (*func)(struct callback_head *);
};

struct fib_config {
	u8 fc_dst_len;
	dscp_t fc_dscp;
	u8 fc_protocol;
	u8 fc_scope;
	u8 fc_type;
	u8 fc_gw_family;
	u32 fc_table;
	__be32 fc_dst;
	union {
		__be32 fc_gw4;
		struct in6_addr fc_gw6;
	};
	int fc_oif;
	u32 fc_flags;
	u32 fc_priority;
	__be32 fc_prefsrc;
	u32 fc_nh_id;
	struct nlattr *fc_mx;
	struct rtnexthop *fc_mp;
	int fc_mx_len;
	int fc_mp_len;
	u32 fc_flow;
	u32 fc_nlflags;
	struct nl_info fc_nlinfo;
	struct nlattr *fc_encap;
	u16 fc_encap_type;
};

struct fib_table {
	struct hlist_node tb_hlist;
	u32 tb_id;
	int tb_num_default;
	struct callback_head rcu;
	long unsigned int *tb_data;
	long unsigned int __data[0];
};


