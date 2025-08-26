# rdma-core
/rdma-core
```
[drwxr-xr-x  4.0K]  /out
├── [drwxr-xr-x  4.0K]  etc
│   ├── [drwxr-xr-x  4.0K]  infiniband-diags
│   │   ├── [-rw-r--r--   367]  error_thresholds
│   │   └── [-rw-r--r--   609]  ibdiag.conf
│   ├── [-rw-r--r--    28]  iwpmd.conf
│   ├── [drwxr-xr-x  4.0K]  libibverbs.d
│   │   ├── [-rw-r--r--    15]  bnxt_re.driver
│   │   ├── [-rw-r--r--    13]  cxgb4.driver
│   │   ├── [-rw-r--r--    11]  efa.driver
│   │   ├── [-rw-r--r--    13]  erdma.driver
│   │   ├── [-rw-r--r--    17]  hfi1verbs.driver
│   │   ├── [-rw-r--r--    11]  hns.driver
│   │   ├── [-rw-r--r--    18]  ipathverbs.driver
│   │   ├── [-rw-r--r--    13]  irdma.driver
│   │   ├── [-rw-r--r--    12]  mana.driver
│   │   ├── [-rw-r--r--    12]  mlx4.driver
│   │   ├── [-rw-r--r--    12]  mlx5.driver
│   │   ├── [-rw-r--r--    13]  mthca.driver
│   │   ├── [-rw-r--r--    14]  ocrdma.driver
│   │   ├── [-rw-r--r--    12]  qedr.driver
│   │   ├── [-rw-r--r--    11]  rxe.driver
│   │   ├── [-rw-r--r--    11]  siw.driver
│   │   └── [-rw-r--r--    18]  vmw_pvrdma.driver
│   ├── [drwxr-xr-x  4.0K]  modprobe.d
│   │   ├── [-rw-r--r--  1004]  mlx4.conf
│   │   └── [-rw-r--r--    92]  truescale.conf
│   ├── [drwxr-xr-x  4.0K]  rdma
│   │   └── [drwxr-xr-x  4.0K]  modules
│   │       ├── [-rw-r--r--   299]  infiniband.conf
│   │       ├── [-rw-r--r--    74]  iwarp.conf
│   │       ├── [-rw-r--r--    69]  iwpmd.conf
│   │       ├── [-rw-r--r--   253]  opa.conf
│   │       ├── [-rw-r--r--   413]  rdma.conf
│   │       ├── [-rw-r--r--    99]  roce.conf
│   │       └── [-rw-r--r--    75]  srp_daemon.conf
│   └── [-rw-r--r--   570]  srp_daemon.conf
└── [drwxr-xr-x  4.0K]  usr
    ├── [drwxr-xr-x  4.0K]  bin
    │   ├── [-rwxr-xr-x   44K]  cmtime
    │   ├── [-rwxr-xr-x   54K]  ib_acme
    │   ├── [-rwxr-xr-x   19K]  ibv_asyncwatch
    │   ├── [-rwxr-xr-x   18K]  ibv_devices
    │   ├── [-rwxr-xr-x   32K]  ibv_devinfo
    │   ├── [-rwxr-xr-x   39K]  ibv_rc_pingpong
    │   ├── [-rwxr-xr-x   38K]  ibv_srq_pingpong
    │   ├── [-rwxr-xr-x   38K]  ibv_uc_pingpong
    │   ├── [-rwxr-xr-x   38K]  ibv_ud_pingpong
    │   ├── [-rwxr-xr-x   38K]  ibv_xsrq_pingpong
    │   ├── [-rwxr-xr-x   39K]  mckey
    │   ├── [-rwxr-xr-x   30K]  rcopy
    │   ├── [-rwxr-xr-x   19K]  rdma_client
    │   ├── [-rwxr-xr-x   19K]  rdma_server
    │   ├── [-rwxr-xr-x   19K]  rdma_xclient
    │   ├── [-rwxr-xr-x   19K]  rdma_xserver
    │   ├── [-rwxr-xr-x   39K]  riostream
    │   ├── [-rwxr-xr-x   44K]  rping
    │   ├── [-rwxr-xr-x   39K]  rstream
    │   ├── [-rwxr-xr-x   39K]  ucmatose
    │   ├── [-rwxr-xr-x   39K]  udaddy
    │   └── [-rwxr-xr-x   35K]  udpong
    ├── [drwxr-xr-x  4.0K]  include
    │   ├── [drwxr-xr-x  4.0K]  infiniband
    │   │   ├── [-rw-r--r--  4.5K]  acm.h
    │   │   ├── [-rw-r--r--  4.1K]  acm_prov.h
    │   │   ├── [-rw-r--r--  1.9K]  arch.h
    │   │   ├── [-rw-r--r--  2.9K]  efadv.h
    │   │   ├── [-rw-r--r--  1.4K]  hnsdv.h
    │   │   ├── [-rw-r--r--  2.8K]  ib.h
    │   │   ├── [-rw-r--r--  7.8K]  ib_user_ioctl_verbs.h
    │   │   ├── [-rw-r--r--  8.2K]  ibnetdisc.h
    │   │   ├── [-rw-r--r--    36]  ibnetdisc_osd.h
    │   │   ├── [-rw-r--r--   48K]  mad.h
    │   │   ├── [-rw-r--r--    36]  mad_osd.h
    │   │   ├── [-rw-r--r--  1.4K]  manadv.h
    │   │   ├── [-rw-r--r--   13K]  mlx4dv.h
    │   │   ├── [-rw-r--r--  4.3K]  mlx5_api.h
    │   │   ├── [-rw-r--r--  3.5K]  mlx5_user_ioctl_verbs.h
    │   │   ├── [-rw-r--r--   61K]  mlx5dv.h
    │   │   ├── [-rw-r--r--  5.6K]  opcode.h
    │   │   ├── [-rw-r--r--  1.6K]  sa-kern-abi.h
    │   │   ├── [-rw-r--r--  4.0K]  sa.h
    │   │   ├── [-rw-r--r--  1.9K]  tm_types.h
    │   │   ├── [-rw-r--r--  7.6K]  umad.h
    │   │   ├── [-rw-r--r--  2.1K]  umad_cm.h
    │   │   ├── [-rw-r--r--  5.8K]  umad_sa.h
    │   │   ├── [-rw-r--r--  4.9K]  umad_sa_mcm.h
    │   │   ├── [-rw-r--r--  3.8K]  umad_sm.h
    │   │   ├── [-rw-r--r--  1.9K]  umad_str.h
    │   │   ├── [-rw-r--r--  5.5K]  umad_types.h
    │   │   ├── [-rw-r--r--   94K]  verbs.h
    │   │   └── [-rw-r--r--  5.3K]  verbs_api.h
    │   └── [drwxr-xr-x  4.0K]  rdma
    │       ├── [-rw-r--r--  5.3K]  bnxt_re-abi.h
    │       ├── [-rw-r--r--  3.0K]  cxgb4-abi.h
    │       ├── [-rw-r--r--  3.6K]  efa-abi.h
    │       ├── [-rw-r--r--   811]  erdma-abi.h
    │       ├── [drwxr-xr-x  4.0K]  hfi
    │       │   ├── [-rw-r--r--  6.5K]  hfi1_ioctl.h
    │       │   └── [-rw-r--r--  9.1K]  hfi1_user.h
    │       ├── [-rw-r--r--  3.9K]  hns-abi.h
    │       ├── [-rw-r--r--  9.6K]  ib_user_ioctl_cmds.h
    │       ├── [-rw-r--r--  7.8K]  ib_user_ioctl_verbs.h
    │       ├── [-rw-r--r--  8.3K]  ib_user_mad.h
    │       ├── [-rw-r--r--  2.3K]  ib_user_sa.h
    │       ├── [-rw-r--r--   28K]  ib_user_verbs.h
    │       ├── [-rw-r--r--  2.3K]  irdma-abi.h
    │       ├── [-rw-r--r--  1.5K]  mana-abi.h
    │       ├── [-rw-r--r--  5.0K]  mlx4-abi.h
    │       ├── [-rw-r--r--   14K]  mlx5-abi.h
    │       ├── [-rw-r--r--   11K]  mlx5_user_ioctl_cmds.h
    │       ├── [-rw-r--r--  3.5K]  mlx5_user_ioctl_verbs.h
    │       ├── [-rw-r--r--  3.0K]  mthca-abi.h
    │       ├── [-rw-r--r--  4.0K]  ocrdma-abi.h
    │       ├── [-rw-r--r--  4.2K]  qedr-abi.h
    │       ├── [-rw-r--r--   28K]  rdma_cma.h
    │       ├── [-rw-r--r--  6.5K]  rdma_cma_abi.h
    │       ├── [-rw-r--r--   15K]  rdma_netlink.h
    │       ├── [-rw-r--r--  7.0K]  rdma_user_cm.h
    │       ├── [-rw-r--r--  3.7K]  rdma_user_ioctl.h
    │       ├── [-rw-r--r--  2.6K]  rdma_user_ioctl_cmds.h
    │       ├── [-rw-r--r--  5.0K]  rdma_user_rxe.h
    │       ├── [-rw-r--r--  7.6K]  rdma_verbs.h
    │       ├── [-rw-r--r--  3.6K]  rsocket.h
    │       ├── [-rw-r--r--  1.7K]  rvt-abi.h
    │       ├── [-rw-r--r--  3.3K]  siw-abi.h
    │       └── [-rw-r--r--  7.8K]  vmw_pvrdma-abi.h
    ├── [drwxr-xr-x  4.0K]  lib
    │   ├── [drwxr-xr-x  4.0K]  systemd
    │   │   └── [drwxr-xr-x  4.0K]  system
    │   │       ├── [-rw-r--r--   895]  ibacm.service
    │   │       ├── [-rw-r--r--  1.2K]  ibacm.socket
    │   │       ├── [-rw-r--r--  1.1K]  iwpmd.service
    │   │       ├── [-rw-r--r--   580]  rdma-hw.target
    │   │       ├── [-rw-r--r--  1.0K]  rdma-load-modules@.service
    │   │       ├── [-rw-r--r--   902]  rdma-ndd.service
    │   │       ├── [-rw-r--r--   468]  srp_daemon.service
    │   │       └── [-rw-r--r--  1.6K]  srp_daemon_port@.service
    │   └── [drwxr-xr-x  4.0K]  udev
    │       ├── [-rwxr-xr-x   29K]  rdma_rename
    │       └── [drwxr-xr-x  4.0K]  rules.d
    │           ├── [-rw-r--r--   230]  60-rdma-ndd.rules
    │           ├── [-rw-r--r--  1.1K]  60-rdma-persistent-naming.rules
    │           ├── [-rw-r--r--   206]  60-srp_daemon.rules
    │           ├── [-rw-r--r--  1.8K]  75-rdma-description.rules
    │           ├── [-rw-r--r--    77]  90-iwpmd.rules
    │           ├── [-rw-r--r--  1.8K]  90-rdma-hw-modules.rules
    │           ├── [-rw-r--r--   645]  90-rdma-ulp-modules.rules
    │           └── [-rw-r--r--   142]  90-rdma-umad.rules
    ├── [drwxr-xr-x  4.0K]  lib64
    │   ├── [drwxr-xr-x  4.0K]  ibacm
    │   │   └── [-rwxr-xr-x   70K]  libibacmp.so
    │   ├── [lrwxrwxrwx    11]  libefa.so -> libefa.so.1
    │   ├── [lrwxrwxrwx    18]  libefa.so.1 -> libefa.so.1.3.53.0
    │   ├── [-rwxr-xr-x   55K]  libefa.so.1.3.53.0
    │   ├── [lrwxrwxrwx    11]  libhns.so -> libhns.so.1
    │   ├── [lrwxrwxrwx    18]  libhns.so.1 -> libhns.so.1.0.53.0
    │   ├── [-rwxr-xr-x   65K]  libhns.so.1.0.53.0
    │   ├── [lrwxrwxrwx    13]  libibmad.so -> libibmad.so.5
    │   ├── [lrwxrwxrwx    20]  libibmad.so.5 -> libibmad.so.5.3.53.0
    │   ├── [-rwxr-xr-x  128K]  libibmad.so.5.3.53.0
    │   ├── [lrwxrwxrwx    17]  libibnetdisc.so -> libibnetdisc.so.5
    │   ├── [lrwxrwxrwx    24]  libibnetdisc.so.5 -> libibnetdisc.so.5.1.53.0
    │   ├── [-rwxr-xr-x   63K]  libibnetdisc.so.5.1.53.0
    │   ├── [lrwxrwxrwx    14]  libibumad.so -> libibumad.so.3
    │   ├── [lrwxrwxrwx    21]  libibumad.so.3 -> libibumad.so.3.3.53.0
    │   ├── [-rwxr-xr-x   48K]  libibumad.so.3.3.53.0
    │   ├── [drwxr-xr-x  4.0K]  libibverbs
    │   │   ├── [-rwxr-xr-x   45K]  libbnxt_re-rdmav34.so
    │   │   ├── [-rwxr-xr-x   52K]  libcxgb4-rdmav34.so
    │   │   ├── [lrwxrwxrwx    21]  libefa-rdmav34.so -> ../libefa.so.1.3.53.0
    │   │   ├── [-rwxr-xr-x   31K]  liberdma-rdmav34.so
    │   │   ├── [-rwxr-xr-x   32K]  libhfi1verbs-rdmav34.so
    │   │   ├── [lrwxrwxrwx    21]  libhns-rdmav34.so -> ../libhns.so.1.0.53.0
    │   │   ├── [-rwxr-xr-x   32K]  libipathverbs-rdmav34.so
    │   │   ├── [-rwxr-xr-x   50K]  libirdma-rdmav34.so
    │   │   ├── [lrwxrwxrwx    22]  libmana-rdmav34.so -> ../libmana.so.1.0.53.0
    │   │   ├── [lrwxrwxrwx    22]  libmlx4-rdmav34.so -> ../libmlx4.so.1.0.53.0
    │   │   ├── [lrwxrwxrwx    23]  libmlx5-rdmav34.so -> ../libmlx5.so.1.24.53.0
    │   │   ├── [-rwxr-xr-x   45K]  libmthca-rdmav34.so
    │   │   ├── [-rwxr-xr-x   40K]  libocrdma-rdmav34.so
    │   │   ├── [-rwxr-xr-x   57K]  libqedr-rdmav34.so
    │   │   ├── [-rwxr-xr-x   41K]  librxe-rdmav34.so
    │   │   ├── [-rwxr-xr-x   31K]  libsiw-rdmav34.so
    │   │   └── [-rwxr-xr-x   31K]  libvmw_pvrdma-rdmav34.so
    │   ├── [lrwxrwxrwx    15]  libibverbs.so -> libibverbs.so.1
    │   ├── [lrwxrwxrwx    23]  libibverbs.so.1 -> libibverbs.so.1.14.53.0
    │   ├── [-rwxr-xr-x  155K]  libibverbs.so.1.14.53.0
    │   ├── [lrwxrwxrwx    12]  libmana.so -> libmana.so.1
    │   ├── [lrwxrwxrwx    19]  libmana.so.1 -> libmana.so.1.0.53.0
    │   ├── [-rwxr-xr-x   45K]  libmana.so.1.0.53.0
    │   ├── [lrwxrwxrwx    12]  libmlx4.so -> libmlx4.so.1
    │   ├── [lrwxrwxrwx    19]  libmlx4.so.1 -> libmlx4.so.1.0.53.0
    │   ├── [-rwxr-xr-x   60K]  libmlx4.so.1.0.53.0
    │   ├── [lrwxrwxrwx    12]  libmlx5.so -> libmlx5.so.1
    │   ├── [lrwxrwxrwx    20]  libmlx5.so.1 -> libmlx5.so.1.24.53.0
    │   ├── [-rwxr-xr-x  506K]  libmlx5.so.1.24.53.0
    │   ├── [lrwxrwxrwx    14]  librdmacm.so -> librdmacm.so.1
    │   ├── [lrwxrwxrwx    21]  librdmacm.so.1 -> librdmacm.so.1.3.53.0
    │   ├── [-rwxr-xr-x  121K]  librdmacm.so.1.3.53.0
    │   ├── [drwxr-xr-x  4.0K]  pkgconfig
    │   │   ├── [-rw-r--r--   253]  libefa.pc
    │   │   ├── [-rw-r--r--   253]  libhns.pc
    │   │   ├── [-rw-r--r--   256]  libibmad.pc
    │   │   ├── [-rw-r--r--   273]  libibnetdisc.pc
    │   │   ├── [-rw-r--r--   249]  libibumad.pc
    │   │   ├── [-rw-r--r--   252]  libibverbs.pc
    │   │   ├── [-rw-r--r--   255]  libmana.pc
    │   │   ├── [-rw-r--r--   255]  libmlx4.pc
    │   │   ├── [-rw-r--r--   256]  libmlx5.pc
    │   │   └── [-rw-r--r--   259]  librdmacm.pc
    │   ├── [drwxr-xr-x  4.0K]  python3
    │   │   └── [drwxr-xr-x  4.0K]  site-packages
    │   │       └── [drwxr-xr-x  4.0K]  pyverbs
    │   │           ├── [-rw-r--r--     0]  __init__.py
    │   │           ├── [-rwxr-xr-x  168K]  addr.cpython-312.so
    │   │           ├── [-rwxr-xr-x  115K]  base.cpython-312.so
    │   │           ├── [-rwxr-xr-x  143K]  cm_enums.cpython-312.so
    │   │           ├── [-rwxr-xr-x  331K]  cmid.cpython-312.so
    │   │           ├── [-rwxr-xr-x  290K]  cq.cpython-312.so
    │   │           ├── [-rwxr-xr-x  548K]  device.cpython-312.so
    │   │           ├── [-rwxr-xr-x   53K]  dma_util.cpython-312.so
    │   │           ├── [-rwxr-xr-x   84K]  dmabuf.cpython-312.so
    │   │           ├── [-rwxr-xr-x  820K]  enums.cpython-312.so
    │   │           ├── [-rwxr-xr-x  117K]  flow.cpython-312.so
    │   │           ├── [-rwxr-xr-x   58K]  fork.cpython-312.so
    │   │           ├── [-rwxr-xr-x   27K]  libibverbs.cpython-312.so
    │   │           ├── [-rwxr-xr-x  812K]  libibverbs_enums.cpython-312.so
    │   │           ├── [-rwxr-xr-x   27K]  librdmacm.cpython-312.so
    │   │           ├── [-rwxr-xr-x  143K]  librdmacm_enums.cpython-312.so
    │   │           ├── [-rwxr-xr-x   95K]  mem_alloc.cpython-312.so
    │   │           ├── [-rwxr-xr-x  240K]  mr.cpython-312.so
    │   │           ├── [-rwxr-xr-x  157K]  pd.cpython-312.so
    │   │           ├── [drwxr-xr-x  4.0K]  providers
    │   │           │   ├── [drwxr-xr-x  4.0K]  efa
    │   │           │   │   ├── [-rwxr-xr-x   27K]  efa_enums.cpython-312.so
    │   │           │   │   ├── [-rwxr-xr-x  186K]  efadv.cpython-312.so
    │   │           │   │   └── [-rwxr-xr-x   27K]  libefa.cpython-312.so
    │   │           │   └── [drwxr-xr-x  4.0K]  mlx5
    │   │           │       ├── [-rwxr-xr-x  319K]  dr_action.cpython-312.so
    │   │           │       ├── [-rwxr-xr-x  101K]  dr_domain.cpython-312.so
    │   │           │       ├── [-rwxr-xr-x   97K]  dr_matcher.cpython-312.so
    │   │           │       ├── [-rwxr-xr-x   82K]  dr_rule.cpython-312.so
    │   │           │       ├── [-rwxr-xr-x   87K]  dr_table.cpython-312.so
    │   │           │       ├── [-rwxr-xr-x   27K]  libmlx5.cpython-312.so
    │   │           │       ├── [-rwxr-xr-x  346K]  mlx5_enums.cpython-312.so
    │   │           │       ├── [-rwxr-xr-x  108K]  mlx5_vfio.cpython-312.so
    │   │           │       ├── [-rwxr-xr-x  731K]  mlx5dv.cpython-312.so
    │   │           │       ├── [-rwxr-xr-x  204K]  mlx5dv_crypto.cpython-312.so
    │   │           │       ├── [-rwxr-xr-x  181K]  mlx5dv_flow.cpython-312.so
    │   │           │       ├── [-rwxr-xr-x  167K]  mlx5dv_mkey.cpython-312.so
    │   │           │       ├── [-rwxr-xr-x  123K]  mlx5dv_objects.cpython-312.so
    │   │           │       └── [-rwxr-xr-x  117K]  mlx5dv_sched.cpython-312.so
    │   │           ├── [-rw-r--r--  1.5K]  pyverbs_error.py
    │   │           ├── [-rwxr-xr-x  456K]  qp.cpython-312.so
    │   │           ├── [-rwxr-xr-x  300K]  spec.cpython-312.so
    │   │           ├── [-rwxr-xr-x  179K]  srq.cpython-312.so
    │   │           ├── [-rw-r--r--  3.1K]  utils.py
    │   │           ├── [-rwxr-xr-x  184K]  wq.cpython-312.so
    │   │           ├── [-rwxr-xr-x  173K]  wr.cpython-312.so
    │   │           └── [-rwxr-xr-x   93K]  xrcd.cpython-312.so
    │   └── [drwxr-xr-x  4.0K]  rsocket
    │       ├── [-rwxr-xr-x   38K]  librspreload.so
    │       ├── [lrwxrwxrwx    15]  librspreload.so.1 -> librspreload.so
    │       └── [lrwxrwxrwx    15]  librspreload.so.1.0.0 -> librspreload.so
    ├── [drwxr-xr-x  4.0K]  libexec
    │   ├── [drwxr-xr-x  4.0K]  srp_daemon
    │   │   └── [-rwxr-xr-x   277]  start_on_all_ports
    │   └── [-rwxr-xr-x  8.5K]  truescale-serdes.cmds
    ├── [drwxr-xr-x  4.0K]  sbin
    │   ├── [-rwxr-xr-x   10K]  check_lft_balance.pl
    │   ├── [-rwxr-xr-x   48K]  dump_fts
    │   ├── [-rwxr-xr-x   280]  dump_lfts.sh
    │   ├── [-rwxr-xr-x   286]  dump_mfts.sh
    │   ├── [-rwxr-xr-x   78K]  ibacm
    │   ├── [-rwxr-xr-x   39K]  ibaddr
    │   ├── [-rwxr-xr-x   39K]  ibcacheedit
    │   ├── [-rwxr-xr-x   43K]  ibccconfig
    │   ├── [-rwxr-xr-x   48K]  ibccquery
    │   ├── [-rwxr-xr-x  6.3K]  ibfindnodesusing.pl
    │   ├── [-rwxr-xr-x  1006]  ibhosts
    │   ├── [-rwxr-xr-x  7.0K]  ibidsverify.pl
    │   ├── [-rwxr-xr-x   57K]  iblinkinfo
    │   ├── [-rwxr-xr-x   57K]  ibnetdiscover
    │   ├── [-rwxr-xr-x    82]  ibnodes
    │   ├── [-rwxr-xr-x   44K]  ibping
    │   ├── [-rwxr-xr-x   52K]  ibportstate
    │   ├── [-rwxr-xr-x   66K]  ibqueryerrors
    │   ├── [-rwxr-xr-x   48K]  ibroute
    │   ├── [-rwxr-xr-x  1006]  ibrouters
    │   ├── [lrwxrwxrwx    10]  ibsrpdm -> srp_daemon
    │   ├── [-rwxr-xr-x   39K]  ibstat
    │   ├── [-rwxr-xr-x  1.8K]  ibstatus
    │   ├── [-rwxr-xr-x  1.4K]  ibswitches
    │   ├── [-rwxr-xr-x   44K]  ibsysstat
    │   ├── [-rwxr-xr-x   57K]  ibtracert
    │   ├── [-rwxr-xr-x   52K]  iwpmd
    │   ├── [-rwxr-xr-x   57K]  perfquery
    │   ├── [-rwxr-xr-x   24K]  rdma-ndd
    │   ├── [lrwxrwxrwx    10]  run_srp_daemon -> srp_daemon
    │   ├── [-rwxr-xr-x   86K]  saquery
    │   ├── [-rwxr-xr-x   39K]  sminfo
    │   ├── [-rwxr-xr-x   39K]  smpdump
    │   ├── [-rwxr-xr-x   56K]  smpquery
    │   ├── [-rwxr-xr-x   74K]  srp_daemon
    │   ├── [-rwxr-xr-x  2.2K]  srp_daemon.sh
    │   └── [-rwxr-xr-x   43K]  vendstat
    └── [drwxr-xr-x  4.0K]  share
        ├── [drwxr-xr-x  4.0K]  doc
        │   └── [drwxr-xr-x  4.0K]  rdma-core
        │       ├── [-rw-r--r--   688]  70-persistent-ipoib.rules
        │       ├── [-rw-r--r--  5.5K]  MAINTAINERS
        │       ├── [-rw-r--r--  3.7K]  README.md
        │       ├── [-rw-r--r--  5.3K]  ibacm.md
        │       ├── [-rw-r--r--  1.7K]  ibsrpdm.md
        │       ├── [-rw-r--r--  3.0K]  libibverbs.md
        │       ├── [-rw-r--r--  1.3K]  librdmacm.md
        │       ├── [-rw-r--r--   452]  rxe.md
        │       ├── [-rw-r--r--   14K]  tag_matching.md
        │       ├── [drwxr-xr-x  4.0K]  tests
        │       │   ├── [-rw-r--r--  1.6K]  __init__.py
        │       │   ├── [-rw-r--r--  2.0K]  args_parser.py
        │       │   ├── [-rw-r--r--   33K]  base.py
        │       │   ├── [-rw-r--r--  7.0K]  base_rdmacm.py
        │       │   ├── [-rw-r--r--  4.1K]  cuda_utils.py
        │       │   ├── [-rw-r--r--  4.7K]  efa_base.py
        │       │   ├── [-rw-r--r--  3.0K]  irdma_base.py
        │       │   ├── [-rw-r--r--   44K]  mlx5_base.py
        │       │   ├── [-rw-r--r--   86K]  mlx5_prm_structs.py
        │       │   ├── [-rw-r--r--   20K]  rdmacm_utils.py
        │       │   ├── [-rw-r--r--   447]  run_tests.py
        │       │   ├── [-rw-r--r--  3.2K]  test_addr.py
        │       │   ├── [-rw-r--r--  5.3K]  test_atomic.py
        │       │   ├── [-rw-r--r--  4.8K]  test_cq.py
        │       │   ├── [-rw-r--r--   972]  test_cq_events.py
        │       │   ├── [-rw-r--r--  3.9K]  test_cqex.py
        │       │   ├── [-rw-r--r--  2.8K]  test_cuda_dmabuf.py
        │       │   ├── [-rw-r--r--   18K]  test_device.py
        │       │   ├── [-rw-r--r--  4.3K]  test_efa_srd.py
        │       │   ├── [-rw-r--r--  6.2K]  test_efadv.py
        │       │   ├── [-rw-r--r--  5.9K]  test_flow.py
        │       │   ├── [-rw-r--r--  1.0K]  test_fork.py
        │       │   ├── [-rw-r--r--  9.8K]  test_mlx5_cq.py
        │       │   ├── [-rw-r--r--   21K]  test_mlx5_crypto.py
        │       │   ├── [-rw-r--r--  4.1K]  test_mlx5_cuda_umem.py
        │       │   ├── [-rw-r--r--  5.6K]  test_mlx5_dc.py
        │       │   ├── [-rw-r--r--  1.0K]  test_mlx5_devx.py
        │       │   ├── [-rw-r--r--  6.0K]  test_mlx5_dm_ops.py
        │       │   ├── [-rw-r--r--  5.2K]  test_mlx5_dma_memcpy.py
        │       │   ├── [-rw-r--r--   71K]  test_mlx5_dr.py
        │       │   ├── [-rw-r--r--  7.5K]  test_mlx5_flow.py
        │       │   ├── [-rw-r--r--  1.9K]  test_mlx5_huge_page.py
        │       │   ├── [-rw-r--r--  2.7K]  test_mlx5_lag_affinity.py
        │       │   ├── [-rw-r--r--   25K]  test_mlx5_mkey.py
        │       │   ├── [-rw-r--r--  2.3K]  test_mlx5_pp.py
        │       │   ├── [-rw-r--r--  1.6K]  test_mlx5_query_port.py
        │       │   ├── [-rw-r--r--  3.2K]  test_mlx5_raw_wqe.py
        │       │   ├── [-rw-r--r--  8.3K]  test_mlx5_rdmacm.py
        │       │   ├── [-rw-r--r--  4.0K]  test_mlx5_sched.py
        │       │   ├── [-rw-r--r--  7.9K]  test_mlx5_timestamp.py
        │       │   ├── [-rw-r--r--  1.3K]  test_mlx5_uar.py
        │       │   ├── [-rw-r--r--  1.3K]  test_mlx5_udp_sport.py
        │       │   ├── [-rw-r--r--  1.3K]  test_mlx5_var.py
        │       │   ├── [-rw-r--r--   11K]  test_mlx5_vfio.py
        │       │   ├── [-rw-r--r--   28K]  test_mr.py
        │       │   ├── [-rw-r--r--   11K]  test_odp.py
        │       │   ├── [-rw-r--r--  6.7K]  test_parent_domain.py
        │       │   ├── [-rw-r--r--  1.2K]  test_pd.py
        │       │   ├── [-rw-r--r--   16K]  test_qp.py
        │       │   ├── [-rw-r--r--   13K]  test_qpex.py
        │       │   ├── [-rw-r--r--  3.0K]  test_rdmacm.py
        │       │   ├── [-rw-r--r--  1.2K]  test_relaxed_ordering.py
        │       │   ├── [-rw-r--r--  5.5K]  test_rss_traffic.py
        │       │   ├── [-rw-r--r--  3.6K]  test_shared_pd.py
        │       │   ├── [-rw-r--r--  2.5K]  test_srq.py
        │       │   ├── [-rw-r--r--   15K]  test_tag_matching.py
        │       │   └── [-rw-r--r--   69K]  utils.py
        │       └── [-rw-r--r--  7.7K]  udev.md
        └── [drwxr-xr-x  4.0K]  perl5
            └── [-rw-r--r--   14K]  IBswcountlimits.pm

37 directories, 343 files
```