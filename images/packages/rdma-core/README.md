# rdma-core
/rdma-core
```
.
|-- etc
|   |-- infiniband-diags
|   |   |-- error_thresholds
|   |   `-- ibdiag.conf
|   |-- init.d
|   |   |-- ibacm
|   |   |-- iwpmd
|   |   `-- srpd
|   |-- iwpmd.conf
|   |-- libibverbs.d
|   |   |-- bnxt_re.driver
|   |   |-- cxgb4.driver
|   |   |-- efa.driver
|   |   |-- erdma.driver
|   |   |-- hfi1verbs.driver
|   |   |-- hns.driver
|   |   |-- ipathverbs.driver
|   |   |-- irdma.driver
|   |   |-- mana.driver
|   |   |-- mlx4.driver
|   |   |-- mlx5.driver
|   |   |-- mthca.driver
|   |   |-- ocrdma.driver
|   |   |-- qedr.driver
|   |   |-- rxe.driver
|   |   |-- siw.driver
|   |   `-- vmw_pvrdma.driver
|   |-- modprobe.d
|   |   |-- mlx4.conf
|   |   `-- truescale.conf
|   |-- rdma
|   |   `-- modules
|   |       |-- infiniband.conf
|   |       |-- iwarp.conf
|   |       |-- iwpmd.conf
|   |       |-- opa.conf
|   |       |-- rdma.conf
|   |       |-- roce.conf
|   |       `-- srp_daemon.conf
|   `-- srp_daemon.conf
`-- usr
    |-- bin
    |   |-- cmtime
    |   |-- ib_acme
    |   |-- ibv_asyncwatch
    |   |-- ibv_devices
    |   |-- ibv_devinfo
    |   |-- ibv_rc_pingpong
    |   |-- ibv_srq_pingpong
    |   |-- ibv_uc_pingpong
    |   |-- ibv_ud_pingpong
    |   |-- ibv_xsrq_pingpong
    |   |-- mckey
    |   |-- rcopy
    |   |-- rdma_client
    |   |-- rdma_server
    |   |-- rdma_xclient
    |   |-- rdma_xserver
    |   |-- riostream
    |   |-- rping
    |   |-- rstream
    |   |-- ucmatose
    |   |-- udaddy
    |   `-- udpong
    |-- include
    |   |-- infiniband
    |   |   |-- acm.h
    |   |   |-- acm_prov.h
    |   |   |-- arch.h
    |   |   |-- efadv.h
    |   |   |-- hnsdv.h
    |   |   |-- ib.h
    |   |   |-- ib_user_ioctl_verbs.h
    |   |   |-- ibnetdisc.h
    |   |   |-- ibnetdisc_osd.h
    |   |   |-- mad.h
    |   |   |-- mad_osd.h
    |   |   |-- manadv.h
    |   |   |-- mlx4dv.h
    |   |   |-- mlx5_api.h
    |   |   |-- mlx5_user_ioctl_verbs.h
    |   |   |-- mlx5dv.h
    |   |   |-- opcode.h
    |   |   |-- sa-kern-abi.h
    |   |   |-- sa.h
    |   |   |-- tm_types.h
    |   |   |-- umad.h
    |   |   |-- umad_cm.h
    |   |   |-- umad_sa.h
    |   |   |-- umad_sa_mcm.h
    |   |   |-- umad_sm.h
    |   |   |-- umad_str.h
    |   |   |-- umad_types.h
    |   |   |-- verbs.h
    |   |   `-- verbs_api.h
    |   `-- rdma
    |       |-- rdma_cma.h
    |       |-- rdma_cma_abi.h
    |       |-- rdma_verbs.h
    |       `-- rsocket.h
    |-- lib
    |   |-- systemd
    |   |   `-- system
    |   |       |-- ibacm.service
    |   |       |-- ibacm.socket
    |   |       |-- iwpmd.service
    |   |       |-- rdma-hw.target
    |   |       |-- rdma-load-modules@.service
    |   |       |-- rdma-ndd.service
    |   |       |-- srp_daemon.service
    |   |       `-- srp_daemon_port@.service
    |   `-- udev
    |       |-- rdma_rename
    |       `-- rules.d
    |           |-- 60-rdma-ndd.rules
    |           |-- 60-rdma-persistent-naming.rules
    |           |-- 60-srp_daemon.rules
    |           |-- 75-rdma-description.rules
    |           |-- 90-iwpmd.rules
    |           |-- 90-rdma-hw-modules.rules
    |           |-- 90-rdma-ulp-modules.rules
    |           `-- 90-rdma-umad.rules
    |-- lib64
    |   |-- ibacm
    |   |   `-- libibacmp.so
    |   |-- libefa.so -> libefa.so.1
    |   |-- libefa.so.1 -> libefa.so.1.3.57.0
    |   |-- libefa.so.1.3.57.0
    |   |-- libhns.so -> libhns.so.1
    |   |-- libhns.so.1 -> libhns.so.1.0.57.0
    |   |-- libhns.so.1.0.57.0
    |   |-- libibmad.so -> libibmad.so.5
    |   |-- libibmad.so.5 -> libibmad.so.5.5.57.0
    |   |-- libibmad.so.5.5.57.0
    |   |-- libibnetdisc.so -> libibnetdisc.so.5
    |   |-- libibnetdisc.so.5 -> libibnetdisc.so.5.1.57.0
    |   |-- libibnetdisc.so.5.1.57.0
    |   |-- libibumad.so -> libibumad.so.3
    |   |-- libibumad.so.3 -> libibumad.so.3.4.57.0
    |   |-- libibumad.so.3.4.57.0
    |   |-- libibverbs
    |   |   |-- libbnxt_re-rdmav57.so
    |   |   |-- libcxgb4-rdmav57.so
    |   |   |-- libefa-rdmav57.so -> ../libefa.so.1.3.57.0
    |   |   |-- liberdma-rdmav57.so
    |   |   |-- libhfi1verbs-rdmav57.so
    |   |   |-- libhns-rdmav57.so -> ../libhns.so.1.0.57.0
    |   |   |-- libipathverbs-rdmav57.so
    |   |   |-- libirdma-rdmav57.so
    |   |   |-- libmana-rdmav57.so -> ../libmana.so.1.0.57.0
    |   |   |-- libmlx4-rdmav57.so -> ../libmlx4.so.1.0.57.0
    |   |   |-- libmlx5-rdmav57.so -> ../libmlx5.so.1.25.57.0
    |   |   |-- libmthca-rdmav57.so
    |   |   |-- libocrdma-rdmav57.so
    |   |   |-- libqedr-rdmav57.so
    |   |   |-- librxe-rdmav57.so
    |   |   |-- libsiw-rdmav57.so
    |   |   `-- libvmw_pvrdma-rdmav57.so
    |   |-- libibverbs.so -> libibverbs.so.1
    |   |-- libibverbs.so.1 -> libibverbs.so.1.14.57.0
    |   |-- libibverbs.so.1.14.57.0
    |   |-- libmana.so -> libmana.so.1
    |   |-- libmana.so.1 -> libmana.so.1.0.57.0
    |   |-- libmana.so.1.0.57.0
    |   |-- libmlx4.so -> libmlx4.so.1
    |   |-- libmlx4.so.1 -> libmlx4.so.1.0.57.0
    |   |-- libmlx4.so.1.0.57.0
    |   |-- libmlx5.so -> libmlx5.so.1
    |   |-- libmlx5.so.1 -> libmlx5.so.1.25.57.0
    |   |-- libmlx5.so.1.25.57.0
    |   |-- librdmacm.so -> librdmacm.so.1
    |   |-- librdmacm.so.1 -> librdmacm.so.1.3.57.0
    |   |-- librdmacm.so.1.3.57.0
    |   |-- pkgconfig
    |   |   |-- libefa.pc
    |   |   |-- libhns.pc
    |   |   |-- libibmad.pc
    |   |   |-- libibnetdisc.pc
    |   |   |-- libibumad.pc
    |   |   |-- libibverbs.pc
    |   |   |-- libmana.pc
    |   |   |-- libmlx4.pc
    |   |   |-- libmlx5.pc
    |   |   `-- librdmacm.pc
    |   |-- python3
    |   |   `-- site-packages
    |   |       `-- pyverbs
    |   |           |-- __init__.py
    |   |           |-- addr.cpython-312.so
    |   |           |-- base.cpython-312.so
    |   |           |-- cm_enums.cpython-312.so
    |   |           |-- cmid.cpython-312.so
    |   |           |-- cq.cpython-312.so
    |   |           |-- device.cpython-312.so
    |   |           |-- dma_util.cpython-312.so
    |   |           |-- dmabuf.cpython-312.so
    |   |           |-- enums.cpython-312.so
    |   |           |-- flow.cpython-312.so
    |   |           |-- fork.cpython-312.so
    |   |           |-- libibverbs.cpython-312.so
    |   |           |-- libibverbs_enums.cpython-312.so
    |   |           |-- librdmacm.cpython-312.so
    |   |           |-- librdmacm_enums.cpython-312.so
    |   |           |-- mem_alloc.cpython-312.so
    |   |           |-- mr.cpython-312.so
    |   |           |-- pd.cpython-312.so
    |   |           |-- providers
    |   |           |   |-- efa
    |   |           |   |   |-- efa_enums.cpython-312.so
    |   |           |   |   |-- efadv.cpython-312.so
    |   |           |   |   `-- libefa.cpython-312.so
    |   |           |   `-- mlx5
    |   |           |       |-- dr_action.cpython-312.so
    |   |           |       |-- dr_domain.cpython-312.so
    |   |           |       |-- dr_matcher.cpython-312.so
    |   |           |       |-- dr_rule.cpython-312.so
    |   |           |       |-- dr_table.cpython-312.so
    |   |           |       |-- libmlx5.cpython-312.so
    |   |           |       |-- mlx5_enums.cpython-312.so
    |   |           |       |-- mlx5_vfio.cpython-312.so
    |   |           |       |-- mlx5dv.cpython-312.so
    |   |           |       |-- mlx5dv_crypto.cpython-312.so
    |   |           |       |-- mlx5dv_dmabuf.cpython-312.so
    |   |           |       |-- mlx5dv_flow.cpython-312.so
    |   |           |       |-- mlx5dv_mkey.cpython-312.so
    |   |           |       |-- mlx5dv_objects.cpython-312.so
    |   |           |       `-- mlx5dv_sched.cpython-312.so
    |   |           |-- pyverbs_error.py
    |   |           |-- qp.cpython-312.so
    |   |           |-- spec.cpython-312.so
    |   |           |-- srq.cpython-312.so
    |   |           |-- utils.py
    |   |           |-- wq.cpython-312.so
    |   |           |-- wr.cpython-312.so
    |   |           `-- xrcd.cpython-312.so
    |   `-- rsocket
    |       |-- librspreload.so
    |       |-- librspreload.so.1 -> librspreload.so
    |       `-- librspreload.so.1.0.0 -> librspreload.so
    |-- libexec
    |   |-- srp_daemon
    |   |   `-- start_on_all_ports
    |   `-- truescale-serdes.cmds
    |-- sbin
    |   |-- check_lft_balance.pl
    |   |-- dump_fts
    |   |-- dump_lfts.sh
    |   |-- dump_mfts.sh
    |   |-- ibacm
    |   |-- ibaddr
    |   |-- ibcacheedit
    |   |-- ibccconfig
    |   |-- ibccquery
    |   |-- ibfindnodesusing.pl
    |   |-- ibhosts
    |   |-- ibidsverify.pl
    |   |-- iblinkinfo
    |   |-- ibnetdiscover
    |   |-- ibnodes
    |   |-- ibping
    |   |-- ibportstate
    |   |-- ibqueryerrors
    |   |-- ibroute
    |   |-- ibrouters
    |   |-- ibsrpdm -> srp_daemon
    |   |-- ibstat
    |   |-- ibstatus
    |   |-- ibswitches
    |   |-- ibsysstat
    |   |-- ibtracert
    |   |-- iwpmd
    |   |-- perfquery
    |   |-- rdma-ndd
    |   |-- run_srp_daemon -> srp_daemon
    |   |-- saquery
    |   |-- sminfo
    |   |-- smpdump
    |   |-- smpquery
    |   |-- srp_daemon
    |   |-- srp_daemon.sh
    |   `-- vendstat
    `-- share
        |-- doc
        |   `-- rdma-core
        |       |-- 70-persistent-ipoib.rules
        |       |-- MAINTAINERS
        |       |-- README.md
        |       |-- ibacm.md
        |       |-- ibsrpdm.md
        |       |-- libibverbs.md
        |       |-- librdmacm.md
        |       |-- rxe.md
        |       |-- tag_matching.md
        |       |-- tests
        |       |   |-- __init__.py
        |       |   |-- args_parser.py
        |       |   |-- base.py
        |       |   |-- base_rdmacm.py
        |       |   |-- cuda_utils.py
        |       |   |-- efa_base.py
        |       |   |-- irdma_base.py
        |       |   |-- mlx5_base.py
        |       |   |-- mlx5_prm_structs.py
        |       |   |-- rdmacm_utils.py
        |       |   |-- run_tests.py
        |       |   |-- test_addr.py
        |       |   |-- test_atomic.py
        |       |   |-- test_cq.py
        |       |   |-- test_cq_events.py
        |       |   |-- test_cqex.py
        |       |   |-- test_cuda_dmabuf.py
        |       |   |-- test_device.py
        |       |   |-- test_efa_srd.py
        |       |   |-- test_efadv.py
        |       |   |-- test_flow.py
        |       |   |-- test_fork.py
        |       |   |-- test_mlx5_cq.py
        |       |   |-- test_mlx5_crypto.py
        |       |   |-- test_mlx5_cuda_umem.py
        |       |   |-- test_mlx5_dc.py
        |       |   |-- test_mlx5_devx.py
        |       |   |-- test_mlx5_dm_ops.py
        |       |   |-- test_mlx5_dma_memcpy.py
        |       |   |-- test_mlx5_dmabuf.py
        |       |   |-- test_mlx5_dr.py
        |       |   |-- test_mlx5_flow.py
        |       |   |-- test_mlx5_huge_page.py
        |       |   |-- test_mlx5_lag_affinity.py
        |       |   |-- test_mlx5_mkey.py
        |       |   |-- test_mlx5_ooo_qp.py
        |       |   |-- test_mlx5_pp.py
        |       |   |-- test_mlx5_query_port.py
        |       |   |-- test_mlx5_raw_wqe.py
        |       |   |-- test_mlx5_rdma_ctrl.py
        |       |   |-- test_mlx5_rdmacm.py
        |       |   |-- test_mlx5_sched.py
        |       |   |-- test_mlx5_timestamp.py
        |       |   |-- test_mlx5_uar.py
        |       |   |-- test_mlx5_udp_sport.py
        |       |   |-- test_mlx5_var.py
        |       |   |-- test_mlx5_vfio.py
        |       |   |-- test_mr.py
        |       |   |-- test_odp.py
        |       |   |-- test_parent_domain.py
        |       |   |-- test_pd.py
        |       |   |-- test_qp.py
        |       |   |-- test_qpex.py
        |       |   |-- test_rdmacm.py
        |       |   |-- test_relaxed_ordering.py
        |       |   |-- test_rss_traffic.py
        |       |   |-- test_shared_pd.py
        |       |   |-- test_srq.py
        |       |   |-- test_tag_matching.py
        |       |   `-- utils.py
        |       `-- udev.md
        `-- perl5
            `-- IBswcountlimits.pm

37 directories, 321 files
```