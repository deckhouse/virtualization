# selinux
/selinux
```
[drwxr-xr-x  4.0K]  ./
├── [drwxr-xr-x  4.0K]  etc/
│   ├── [drwxr-xr-x  4.0K]  dbus-1/
│   │   └── [drwxr-xr-x  4.0K]  system.d/
│   │       └── [-rw-r--r--   535]  org.selinux.conf
│   ├── [drwxr-xr-x  4.0K]  pam.d/
│   │   ├── [-rw-r--r--   284]  newrole
│   │   └── [-rw-r--r--   283]  run_init
│   ├── [drwxr-xr-x  4.0K]  rc.d/
│   │   └── [drwxr-xr-x  4.0K]  init.d/
│   │       ├── [-rwxr-xr-x  1.7K]  mcstrans*
│   │       └── [-rwxr-xr-x  1.8K]  restorecond*
│   ├── [drwxr-xr-x  4.0K]  selinux/
│   │   ├── [-rw-r--r--   118]  restorecond.conf
│   │   ├── [-rw-r--r--    93]  restorecond_user.conf
│   │   └── [-rw-r--r--  1.9K]  semanage.conf
│   ├── [-rw-r--r--   216]  sestatus.conf
│   ├── [drwxr-xr-x  4.0K]  sysconfig/
│   │   └── [-rw-r--r--    85]  sandbox
│   └── [drwxr-xr-x  4.0K]  xdg/
│       └── [drwxr-xr-x  4.0K]  autostart/
│           └── [-rw-r--r--   222]  restorecond.desktop
├── [drwxr-xr-x  4.0K]  usr/
│   ├── [drwxr-xr-x  4.0K]  bin/
│   │   ├── [-rwxr-xr-x   15K]  audit2allow*
│   │   ├── [lrwxrwxrwx    11]  audit2why -> audit2allow*
│   │   ├── [-rwxr-xr-x   14K]  chcat*
│   │   ├── [-rwxr-xr-x  443K]  checkmodule*
│   │   ├── [-rwxr-xr-x  515K]  checkpolicy*
│   │   ├── [-rwxr-xr-x   15K]  chkcon*
│   │   ├── [-r-xr-xr-x   31K]  newrole*
│   │   ├── [-rwxr-xr-x   18K]  sandbox*
│   │   ├── [-rwxr-xr-x   15K]  secil2conf*
│   │   ├── [-rwxr-xr-x   15K]  secil2tree*
│   │   ├── [-rwxr-xr-x   27K]  secilc*
│   │   ├── [-rwxr-xr-x   28K]  secon*
│   │   ├── [-rwxr-xr-x   33K]  selinux-polgengui*
│   │   ├── [-rwxr-xr-x   15K]  semodule_expand*
│   │   ├── [-rwxr-xr-x   15K]  semodule_link*
│   │   ├── [-rwxr-xr-x   15K]  semodule_package*
│   │   ├── [-rwxr-xr-x   15K]  semodule_unpackage*
│   │   ├── [-rwxr-xr-x   15K]  sepol_check_access*
│   │   ├── [-rwxr-xr-x   15K]  sepol_compute_av*
│   │   ├── [-rwxr-xr-x   15K]  sepol_compute_member*
│   │   ├── [-rwxr-xr-x   15K]  sepol_compute_relabel*
│   │   ├── [-rwxr-xr-x   15K]  sepol_validate_transition*
│   │   ├── [lrwxrwxrwx     8]  sepolgen -> sepolicy*
│   │   ├── [-rwxr-xr-x  4.2K]  sepolgen-ifgen*
│   │   ├── [-rwxr-xr-x  235K]  sepolgen-ifgen-attr-helper*
│   │   ├── [-rwxr-xr-x   29K]  sepolicy*
│   │   ├── [-rwxr-xr-x   23K]  sestatus*
│   │   └── [-rwxr-xr-x    90]  system-config-selinux*
│   ├── [drwxr-xr-x  4.0K]  include/
│   │   ├── [drwxr-xr-x  4.0K]  selinux/
│   │   │   ├── [-rw-r--r--   16K]  avc.h
│   │   │   ├── [-rw-r--r--  1.2K]  context.h
│   │   │   ├── [-rw-r--r--  2.9K]  get_context_list.h
│   │   │   ├── [-rw-r--r--   643]  get_default_type.h
│   │   │   ├── [-rw-r--r--  6.3K]  label.h
│   │   │   ├── [-rw-r--r--  7.3K]  restorecon.h
│   │   │   └── [-rw-r--r--   28K]  selinux.h
│   │   ├── [drwxr-xr-x  4.0K]  semanage/
│   │   │   ├── [-rw-r--r--  1.6K]  boolean_record.h
│   │   │   ├── [-rw-r--r--  1.0K]  booleans_active.h
│   │   │   ├── [-rw-r--r--  1.1K]  booleans_local.h
│   │   │   ├── [-rw-r--r--   820]  booleans_policy.h
│   │   │   ├── [-rw-r--r--  1.8K]  context_record.h
│   │   │   ├── [-rw-r--r--  1.8K]  debug.h
│   │   │   ├── [-rw-r--r--  2.4K]  fcontext_record.h
│   │   │   ├── [-rw-r--r--  1.2K]  fcontexts_local.h
│   │   │   ├── [-rw-r--r--  1020]  fcontexts_policy.h
│   │   │   ├── [-rw-r--r--  7.3K]  handle.h
│   │   │   ├── [-rw-r--r--  2.1K]  ibendport_record.h
│   │   │   ├── [-rw-r--r--  1.2K]  ibendports_local.h
│   │   │   ├── [-rw-r--r--   896]  ibendports_policy.h
│   │   │   ├── [-rw-r--r--  2.4K]  ibpkey_record.h
│   │   │   ├── [-rw-r--r--  1.1K]  ibpkeys_local.h
│   │   │   ├── [-rw-r--r--   829]  ibpkeys_policy.h
│   │   │   ├── [-rw-r--r--  1.9K]  iface_record.h
│   │   │   ├── [-rw-r--r--  1.1K]  interfaces_local.h
│   │   │   ├── [-rw-r--r--   834]  interfaces_policy.h
│   │   │   ├── [-rw-r--r--  9.9K]  modules.h
│   │   │   ├── [-rw-r--r--  2.8K]  node_record.h
│   │   │   ├── [-rw-r--r--  1.1K]  nodes_local.h
│   │   │   ├── [-rw-r--r--   811]  nodes_policy.h
│   │   │   ├── [-rw-r--r--  2.1K]  port_record.h
│   │   │   ├── [-rw-r--r--  1.1K]  ports_local.h
│   │   │   ├── [-rw-r--r--   811]  ports_policy.h
│   │   │   ├── [-rw-r--r--  2.1K]  semanage.h
│   │   │   ├── [-rw-r--r--  1.9K]  seuser_record.h
│   │   │   ├── [-rw-r--r--  1.1K]  seusers_local.h
│   │   │   ├── [-rw-r--r--   835]  seusers_policy.h
│   │   │   ├── [-rw-r--r--  2.7K]  user_record.h
│   │   │   ├── [-rw-r--r--  1.1K]  users_local.h
│   │   │   └── [-rw-r--r--   811]  users_policy.h
│   │   └── [drwxr-xr-x  4.0K]  sepol/
│   │       ├── [-rw-r--r--  1.5K]  boolean_record.h
│   │       ├── [-rw-r--r--  1.3K]  booleans.h
│   │       ├── [drwxr-xr-x  4.0K]  cil/
│   │       │   └── [-rw-r--r--  3.7K]  cil.h
│   │       ├── [-rw-r--r--   752]  context.h
│   │       ├── [-rw-r--r--  1.6K]  context_record.h
│   │       ├── [-rw-r--r--   975]  debug.h
│   │       ├── [-rw-r--r--   826]  errcodes.h
│   │       ├── [-rw-r--r--  1.4K]  handle.h
│   │       ├── [-rw-r--r--  2.1K]  ibendport_record.h
│   │       ├── [-rw-r--r--  1.4K]  ibendports.h
│   │       ├── [-rw-r--r--  2.2K]  ibpkey_record.h
│   │       ├── [-rw-r--r--  1.3K]  ibpkeys.h
│   │       ├── [-rw-r--r--  1.8K]  iface_record.h
│   │       ├── [-rw-r--r--  1.4K]  interfaces.h
│   │       ├── [-rw-r--r--   125]  kernel_to_cil.h
│   │       ├── [-rw-r--r--   126]  kernel_to_conf.h
│   │       ├── [-rw-r--r--  2.6K]  module.h
│   │       ├── [-rw-r--r--   329]  module_to_cil.h
│   │       ├── [-rw-r--r--  2.7K]  node_record.h
│   │       ├── [-rw-r--r--  1.3K]  nodes.h
│   │       ├── [drwxr-xr-x  4.0K]  policydb/
│   │       │   ├── [-rw-r--r--  1.6K]  avrule_block.h
│   │       │   ├── [-rw-r--r--  4.7K]  avtab.h
│   │       │   ├── [-rw-r--r--  4.7K]  conditional.h
│   │       │   ├── [-rw-r--r--  2.5K]  constraint.h
│   │       │   ├── [-rw-r--r--  3.5K]  context.h
│   │       │   ├── [-rw-r--r--  3.5K]  ebitmap.h
│   │       │   ├── [-rw-r--r--  3.6K]  expand.h
│   │       │   ├── [-rw-r--r--  1.5K]  flask_types.h
│   │       │   ├── [-rw-r--r--  3.3K]  hashtab.h
│   │       │   ├── [-rw-r--r--  1.8K]  hierarchy.h
│   │       │   ├── [-rw-r--r--   517]  link.h
│   │       │   ├── [-rw-r--r--  5.0K]  mls_types.h
│   │       │   ├── [-rw-r--r--  1.5K]  module.h
│   │       │   ├── [-rw-r--r--   772]  polcaps.h
│   │       │   ├── [-rw-r--r--   26K]  policydb.h
│   │       │   ├── [-rw-r--r--  8.5K]  services.h
│   │       │   ├── [-rw-r--r--  1.9K]  sidtab.h
│   │       │   ├── [-rw-r--r--  1.1K]  symtab.h
│   │       │   └── [-rw-r--r--  1.5K]  util.h
│   │       ├── [-rw-r--r--  4.7K]  policydb.h
│   │       ├── [-rw-r--r--  2.0K]  port_record.h
│   │       ├── [-rw-r--r--  1.3K]  ports.h
│   │       ├── [-rw-r--r--   862]  sepol.h
│   │       ├── [-rw-r--r--  2.3K]  user_record.h
│   │       └── [-rw-r--r--  1.3K]  users.h
│   ├── [drwxr-xr-x  4.0K]  lib/
│   │   ├── [drwxr-xr-x  4.0K]  python3/
│   │   │   └── [drwxr-xr-x  4.0K]  site-packages/
│   │   │       ├── [-rw-r--r--  105K]  seobject.py
│   │   │       ├── [drwxr-xr-x  4.0K]  sepolgen/
│   │   │       │   ├── [-rw-r--r--     0]  __init__.py
│   │   │       │   ├── [-rw-r--r--   12K]  access.py
│   │   │       │   ├── [-rw-r--r--   21K]  audit.py
│   │   │       │   ├── [-rw-r--r--  2.8K]  classperms.py
│   │   │       │   ├── [-rw-r--r--  2.8K]  defaults.py
│   │   │       │   ├── [-rw-r--r--   16K]  interfaces.py
│   │   │       │   ├── [-rw-r--r--   42K]  lex.py
│   │   │       │   ├── [-rw-r--r--  8.5K]  matching.py
│   │   │       │   ├── [-rw-r--r--  7.1K]  module.py
│   │   │       │   ├── [-rw-r--r--  6.4K]  objectmodel.py
│   │   │       │   ├── [-rw-r--r--  5.0K]  output.py
│   │   │       │   ├── [-rw-r--r--   15K]  policygen.py
│   │   │       │   ├── [-rw-r--r--   31K]  refparser.py
│   │   │       │   ├── [-rw-r--r--   31K]  refpolicy.py
│   │   │       │   ├── [-rw-r--r--  1013]  sepolgeni18n.py
│   │   │       │   ├── [-rw-r--r--  5.4K]  util.py
│   │   │       │   └── [-rw-r--r--  134K]  yacc.py
│   │   │       ├── [drwxr-xr-x  4.0K]  sepolicy/
│   │   │       │   ├── [-rw-r--r--   37K]  __init__.py
│   │   │       │   ├── [drwxr-xr-x  4.0K]  __pycache__/
│   │   │       │   │   ├── [-rw-r--r--   54K]  __init__.cpython-312.pyc
│   │   │       │   │   ├── [-rw-r--r--  1.6K]  booleans.cpython-312.pyc
│   │   │       │   │   ├── [-rw-r--r--  2.1K]  communicate.cpython-312.pyc
│   │   │       │   │   ├── [-rw-r--r--   79K]  generate.cpython-312.pyc
│   │   │       │   │   ├── [-rw-r--r--  182K]  gui.cpython-312.pyc
│   │   │       │   │   ├── [-rw-r--r--   10K]  interface.cpython-312.pyc
│   │   │       │   │   ├── [-rw-r--r--   55K]  manpage.cpython-312.pyc
│   │   │       │   │   ├── [-rw-r--r--  2.8K]  network.cpython-312.pyc
│   │   │       │   │   ├── [-rw-r--r--  3.0K]  sedbus.cpython-312.pyc
│   │   │       │   │   └── [-rw-r--r--  5.0K]  transition.cpython-312.pyc
│   │   │       │   ├── [-rw-r--r--  1.5K]  booleans.py
│   │   │       │   ├── [-rw-r--r--  1.7K]  communicate.py
│   │   │       │   ├── [-rw-r--r--   50K]  generate.py
│   │   │       │   ├── [-rw-r--r--  131K]  gui.py
│   │   │       │   ├── [drwxr-xr-x  4.0K]  help/
│   │   │       │   │   ├── [-rw-r--r--     0]  __init__.py
│   │   │       │   │   ├── [drwxr-xr-x  4.0K]  __pycache__/
│   │   │       │   │   │   └── [-rw-r--r--   157]  __init__.cpython-312.pyc
│   │   │       │   │   ├── [-rw-r--r--   71K]  booleans.png
│   │   │       │   │   ├── [-rw-r--r--   478]  booleans.txt
│   │   │       │   │   ├── [-rw-r--r--   61K]  booleans_more.png
│   │   │       │   │   ├── [-rw-r--r--   193]  booleans_more.txt
│   │   │       │   │   ├── [-rw-r--r--   34K]  booleans_more_show.png
│   │   │       │   │   ├── [-rw-r--r--    62]  booleans_more_show.txt
│   │   │       │   │   ├── [-rw-r--r--   61K]  booleans_toggled.png
│   │   │       │   │   ├── [-rw-r--r--   310]  booleans_toggled.txt
│   │   │       │   │   ├── [-rw-r--r--   48K]  file_equiv.png
│   │   │       │   │   ├── [-rw-r--r--  1.2K]  file_equiv.txt
│   │   │       │   │   ├── [-rw-r--r--   80K]  files_apps.png
│   │   │       │   │   ├── [-rw-r--r--   563]  files_apps.txt
│   │   │       │   │   ├── [-rw-r--r--   66K]  files_exec.png
│   │   │       │   │   ├── [-rw-r--r--   398]  files_exec.txt
│   │   │       │   │   ├── [-rw-r--r--   76K]  files_write.png
│   │   │       │   │   ├── [-rw-r--r--   567]  files_write.txt
│   │   │       │   │   ├── [-rw-r--r--   49K]  lockdown.png
│   │   │       │   │   ├── [-rw-r--r--   291]  lockdown.txt
│   │   │       │   │   ├── [-rw-r--r--   29K]  lockdown_permissive.png
│   │   │       │   │   ├── [-rw-r--r--   722]  lockdown_permissive.txt
│   │   │       │   │   ├── [-rw-r--r--   29K]  lockdown_ptrace.png
│   │   │       │   │   ├── [-rw-r--r--  1.2K]  lockdown_ptrace.txt
│   │   │       │   │   ├── [-rw-r--r--   27K]  lockdown_unconfined.png
│   │   │       │   │   ├── [-rw-r--r--   867]  lockdown_unconfined.txt
│   │   │       │   │   ├── [-rw-r--r--   39K]  login.png
│   │   │       │   │   ├── [-rw-r--r--   786]  login.txt
│   │   │       │   │   ├── [-rw-r--r--   41K]  login_default.png
│   │   │       │   │   ├── [-rw-r--r--   507]  login_default.txt
│   │   │       │   │   ├── [-rw-r--r--   58K]  ports_inbound.png
│   │   │       │   │   ├── [-rw-r--r--   336]  ports_inbound.txt
│   │   │       │   │   ├── [-rw-r--r--   52K]  ports_outbound.png
│   │   │       │   │   ├── [-rw-r--r--   346]  ports_outbound.txt
│   │   │       │   │   ├── [-rw-r--r--   14K]  start.png
│   │   │       │   │   ├── [-rw-r--r--   505]  start.txt
│   │   │       │   │   ├── [-rw-r--r--   49K]  system.png
│   │   │       │   │   ├── [-rw-r--r--    81]  system.txt
│   │   │       │   │   ├── [-rw-r--r--   51K]  system_boot_mode.png
│   │   │       │   │   ├── [-rw-r--r--   458]  system_boot_mode.txt
│   │   │       │   │   ├── [-rw-r--r--   51K]  system_current_mode.png
│   │   │       │   │   ├── [-rw-r--r--   344]  system_current_mode.txt
│   │   │       │   │   ├── [-rw-r--r--   52K]  system_export.png
│   │   │       │   │   ├── [-rw-r--r--   416]  system_export.txt
│   │   │       │   │   ├── [-rw-r--r--   53K]  system_policy_type.png
│   │   │       │   │   ├── [-rw-r--r--   410]  system_policy_type.txt
│   │   │       │   │   ├── [-rw-r--r--   52K]  system_relabel.png
│   │   │       │   │   ├── [-rw-r--r--   399]  system_relabel.txt
│   │   │       │   │   ├── [-rw-r--r--   68K]  transition_file.png
│   │   │       │   │   ├── [-rw-r--r--  1.0K]  transition_file.txt
│   │   │       │   │   ├── [-rw-r--r--   62K]  transition_from.png
│   │   │       │   │   ├── [-rw-r--r--   619]  transition_from.txt
│   │   │       │   │   ├── [-rw-r--r--   66K]  transition_from_boolean.png
│   │   │       │   │   ├── [-rw-r--r--   463]  transition_from_boolean.txt
│   │   │       │   │   ├── [-rw-r--r--   70K]  transition_from_boolean_1.png
│   │   │       │   │   ├── [-rw-r--r--   235]  transition_from_boolean_1.txt
│   │   │       │   │   ├── [-rw-r--r--   31K]  transition_from_boolean_2.png
│   │   │       │   │   ├── [-rw-r--r--   132]  transition_from_boolean_2.txt
│   │   │       │   │   ├── [-rw-r--r--   58K]  transition_to.png
│   │   │       │   │   ├── [-rw-r--r--   605]  transition_to.txt
│   │   │       │   │   ├── [-rw-r--r--   56K]  users.png
│   │   │       │   │   └── [-rw-r--r--   814]  users.txt
│   │   │       │   ├── [-rw-r--r--  8.0K]  interface.py
│   │   │       │   ├── [-rw-r--r--   39K]  manpage.py
│   │   │       │   ├── [-rw-r--r--  2.7K]  network.py
│   │   │       │   ├── [-rw-r--r--  1.5K]  sedbus.py
│   │   │       │   ├── [-rw-r--r--  307K]  sepolicy.glade
│   │   │       │   ├── [drwxr-xr-x  4.0K]  templates/
│   │   │       │   │   ├── [-rw-r--r--   724]  __init__.py
│   │   │       │   │   ├── [drwxr-xr-x  4.0K]  __pycache__/
│   │   │       │   │   │   ├── [-rw-r--r--   162]  __init__.cpython-312.pyc
│   │   │       │   │   │   ├── [-rw-r--r--   334]  boolean.cpython-312.pyc
│   │   │       │   │   │   ├── [-rw-r--r--  2.8K]  etc_rw.cpython-312.pyc
│   │   │       │   │   │   ├── [-rw-r--r--  8.9K]  executable.cpython-312.pyc
│   │   │       │   │   │   ├── [-rw-r--r--   13K]  network.cpython-312.pyc
│   │   │       │   │   │   ├── [-rw-r--r--  2.9K]  rw.cpython-312.pyc
│   │   │       │   │   │   ├── [-rw-r--r--  3.4K]  script.cpython-312.pyc
│   │   │       │   │   │   ├── [-rw-r--r--   476]  semodule.cpython-312.pyc
│   │   │       │   │   │   ├── [-rw-r--r--  2.4K]  spec.cpython-312.pyc
│   │   │       │   │   │   ├── [-rw-r--r--  2.9K]  test_module.cpython-312.pyc
│   │   │       │   │   │   ├── [-rw-r--r--  2.6K]  tmp.cpython-312.pyc
│   │   │       │   │   │   ├── [-rw-r--r--  1.2K]  unit_file.cpython-312.pyc
│   │   │       │   │   │   ├── [-rw-r--r--  3.6K]  user.cpython-312.pyc
│   │   │       │   │   │   ├── [-rw-r--r--  3.1K]  var_cache.cpython-312.pyc
│   │   │       │   │   │   ├── [-rw-r--r--  3.2K]  var_lib.cpython-312.pyc
│   │   │       │   │   │   ├── [-rw-r--r--  2.2K]  var_log.cpython-312.pyc
│   │   │       │   │   │   ├── [-rw-r--r--  2.1K]  var_run.cpython-312.pyc
│   │   │       │   │   │   └── [-rw-r--r--  3.0K]  var_spool.cpython-312.pyc
│   │   │       │   │   ├── [-rw-r--r--  1.2K]  boolean.py
│   │   │       │   │   ├── [-rw-r--r--  3.8K]  etc_rw.py
│   │   │       │   │   ├── [-rw-r--r--  9.7K]  executable.py
│   │   │       │   │   ├── [-rw-r--r--   13K]  network.py
│   │   │       │   │   ├── [-rw-r--r--  3.8K]  rw.py
│   │   │       │   │   ├── [-rw-r--r--  4.2K]  script.py
│   │   │       │   │   ├── [-rw-r--r--  1.3K]  semodule.py
│   │   │       │   │   ├── [-rw-r--r--  2.2K]  spec.py
│   │   │       │   │   ├── [-rw-r--r--  4.3K]  test_module.py
│   │   │       │   │   ├── [-rw-r--r--  3.4K]  tmp.py
│   │   │       │   │   ├── [-rw-r--r--  2.2K]  unit_file.py
│   │   │       │   │   ├── [-rw-r--r--  4.3K]  user.py
│   │   │       │   │   ├── [-rw-r--r--  4.1K]  var_cache.py
│   │   │       │   │   ├── [-rw-r--r--  4.2K]  var_lib.py
│   │   │       │   │   ├── [-rw-r--r--  3.2K]  var_log.py
│   │   │       │   │   ├── [-rw-r--r--  2.9K]  var_run.py
│   │   │       │   │   └── [-rw-r--r--  4.0K]  var_spool.py
│   │   │       │   └── [-rw-r--r--  3.1K]  transition.py
│   │   │       └── [drwxr-xr-x  4.0K]  sepolicy-3.6.dist-info/
│   │   │           ├── [-rw-r--r--     4]  INSTALLER
│   │   │           ├── [-rw-r--r--   207]  METADATA
│   │   │           ├── [-rw-r--r--  9.8K]  RECORD
│   │   │           ├── [-rw-r--r--     0]  REQUESTED
│   │   │           ├── [-rw-r--r--    91]  WHEEL
│   │   │           ├── [-rw-r--r--    54]  direct_url.json
│   │   │           └── [-rw-r--r--     9]  top_level.txt
│   │   └── [drwxr-xr-x  4.0K]  systemd/
│   │       ├── [drwxr-xr-x  4.0K]  system/
│   │       │   ├── [-rw-r--r--   353]  mcstrans.service
│   │       │   └── [-rw-r--r--   292]  restorecond.service
│   │       └── [drwxr-xr-x  4.0K]  user/
│   │           └── [-rw-r--r--   277]  restorecond_user.service
│   ├── [drwxr-xr-x  4.0K]  lib64/
│   │   ├── [-rw-r--r--  410K]  libselinux.a
│   │   ├── [lrwxrwxrwx    15]  libselinux.so -> libselinux.so.1*
│   │   ├── [-rwxr-xr-x  180K]  libselinux.so.1*
│   │   ├── [-rw-r--r--  567K]  libsemanage.a
│   │   ├── [lrwxrwxrwx    16]  libsemanage.so -> libsemanage.so.2*
│   │   ├── [-rwxr-xr-x  268K]  libsemanage.so.2*
│   │   ├── [-rw-r--r--  1.5M]  libsepol.a
│   │   ├── [lrwxrwxrwx    13]  libsepol.so -> libsepol.so.2*
│   │   ├── [-rwxr-xr-x  772K]  libsepol.so.2*
│   │   ├── [drwxr-xr-x  4.0K]  pkgconfig/
│   │   │   ├── [-rw-r--r--   276]  libselinux.pc
│   │   │   ├── [-rw-r--r--   301]  libsemanage.pc
│   │   │   └── [-rw-r--r--   233]  libsepol.pc
│   │   └── [drwxr-xr-x  4.0K]  python3/
│   │       └── [drwxr-xr-x  4.0K]  site-packages/
│   │           ├── [lrwxrwxrwx    31]  _selinux.cpython-312.so -> selinux/_selinux.cpython-312.so*
│   │           ├── [-rwxr-xr-x  303K]  _semanage.cpython-312.so*
│   │           ├── [drwxr-xr-x  4.0K]  selinux/
│   │           │   ├── [-rw-r--r--   38K]  __init__.py
│   │           │   ├── [-rwxr-xr-x  263K]  _selinux.cpython-312.so*
│   │           │   └── [-rwxr-xr-x  243K]  audit2why.cpython-312.so*
│   │           ├── [drwxr-xr-x  4.0K]  selinux-3.6.dist-info/
│   │           │   ├── [-rw-r--r--     4]  INSTALLER
│   │           │   ├── [-rw-r--r--   201]  METADATA
│   │           │   ├── [-rw-r--r--   742]  RECORD
│   │           │   ├── [-rw-r--r--     0]  REQUESTED
│   │           │   ├── [-rw-r--r--   104]  WHEEL
│   │           │   ├── [-rw-r--r--    53]  direct_url.json
│   │           │   └── [-rw-r--r--     8]  top_level.txt
│   │           └── [-rw-r--r--   38K]  semanage.py
│   ├── [drwxr-xr-x  4.0K]  libexec/
│   │   └── [drwxr-xr-x  4.0K]  selinux/
│   │       ├── [drwxr-xr-x  4.0K]  hll/
│   │       │   └── [-rwxr-xr-x   15K]  pp*
│   │       └── [-rwxr-xr-x  9.0K]  semanage_migrate_store*
│   ├── [drwxr-xr-x  4.0K]  sbin/
│   │   ├── [-rwxr-xr-x   15K]  avcstat*
│   │   ├── [-rwxr-xr-x   15K]  compute_av*
│   │   ├── [-rwxr-xr-x   15K]  compute_create*
│   │   ├── [-rwxr-xr-x   15K]  compute_member*
│   │   ├── [-rwxr-xr-x   15K]  compute_relabel*
│   │   ├── [-rwxr-xr-x   12K]  fixfiles*
│   │   ├── [lrwxrwxrwx     8]  genhomedircon -> semodule*
│   │   ├── [-rwxr-xr-x   15K]  getconlist*
│   │   ├── [-rwxr-xr-x   15K]  getdefaultcon*
│   │   ├── [-rwxr-xr-x   15K]  getenforce*
│   │   ├── [-rwxr-xr-x   15K]  getfilecon*
│   │   ├── [-rwxr-xr-x   15K]  getpidcon*
│   │   ├── [-rwxr-xr-x   15K]  getpidprevcon*
│   │   ├── [-rwxr-xr-x   15K]  getpolicyload*
│   │   ├── [-rwxr-xr-x   15K]  getsebool*
│   │   ├── [-rwxr-xr-x   15K]  getseuser*
│   │   ├── [-rwxr-xr-x   15K]  load_policy*
│   │   ├── [-rwxr-xr-x   15K]  matchpathcon*
│   │   ├── [-rwxr-xr-x  239K]  mcstransd*
│   │   ├── [-rwxr-xr-x   15K]  open_init_pty*
│   │   ├── [-rwxr-xr-x   15K]  policyvers*
│   │   ├── [lrwxrwxrwx     8]  restorecon -> setfiles*
│   │   ├── [-rwxr-xr-x   15K]  restorecon_xattr*
│   │   ├── [-rwxr-xr-x   27K]  restorecond*
│   │   ├── [-rwxr-xr-x   15K]  run_init*
│   │   ├── [-rwxr-xr-x   75K]  sefcontext_compile*
│   │   ├── [-rwxr-xr-x   15K]  selabel_digest*
│   │   ├── [-rwxr-xr-x   15K]  selabel_get_digests_all_partial_matches*
│   │   ├── [-rwxr-xr-x   15K]  selabel_lookup*
│   │   ├── [-rwxr-xr-x   15K]  selabel_lookup_best_match*
│   │   ├── [-rwxr-xr-x   15K]  selabel_partial_match*
│   │   ├── [-rwxr-xr-x   15K]  selinux_check_access*
│   │   ├── [-rwxr-xr-x   15K]  selinux_check_securetty_context*
│   │   ├── [-rwxr-xr-x   15K]  selinuxenabled*
│   │   ├── [-rwxr-xr-x   15K]  selinuxexeccon*
│   │   ├── [-rwxr-xr-x   41K]  semanage*
│   │   ├── [-rwxr-xr-x   27K]  semodule*
│   │   ├── [lrwxrwxrwx    15]  sestatus -> ../bin/sestatus*
│   │   ├── [-rwxr-xr-x   15K]  setenforce*
│   │   ├── [-rwxr-xr-x   15K]  setfilecon*
│   │   ├── [-rwxr-xr-x   23K]  setfiles*
│   │   ├── [-rwxr-xr-x   19K]  setsebool*
│   │   ├── [-rwsr-xr-x   31K]  seunshare*
│   │   ├── [-rwxr-xr-x   15K]  togglesebool*
│   │   └── [-rwxr-xr-x   15K]  validatetrans*
│   └── [drwxr-xr-x  4.0K]  share/
│       ├── [drwxr-xr-x  4.0K]  polkit-1/
│       │   └── [drwxr-xr-x  4.0K]  actions/
│       │       ├── [-rw-r--r--   928]  org.selinux.config.policy
│       │       └── [-rw-r--r--  3.2K]  org.selinux.policy
│       ├── [drwxr-xr-x  4.0K]  sandbox/
│       │   ├── [-rwxr-xr-x   991]  sandboxX.sh*
│       │   └── [-rwxr-xr-x   250]  start*
│       └── [drwxr-xr-x  4.0K]  system-config-selinux/
│           ├── [-rw-r--r--  7.8K]  booleansPage.py
│           ├── [-rw-r--r--  5.1K]  domainsPage.py
│           ├── [-rw-r--r--  8.4K]  fcontextPage.py
│           ├── [-rw-r--r--  6.8K]  loginsPage.py
│           ├── [-rw-r--r--  6.8K]  modulesPage.py
│           ├── [-rw-r--r--  137K]  polgen.ui
│           ├── [-rw-r--r--   10K]  portsPage.py
│           ├── [-rwxr-xr-x  6.4K]  selinux_server.py*
│           ├── [-rw-r--r--  5.3K]  semanagePage.py
│           ├── [-rw-r--r--  7.6K]  statusPage.py
│           ├── [-rw-r--r--  1.4K]  system-config-selinux.png
│           ├── [-rwxr-xr-x  6.1K]  system-config-selinux.py*
│           ├── [-rw-r--r--  100K]  system-config-selinux.ui
│           └── [-rw-r--r--  5.3K]  usersPage.py
└── [drwxr-xr-x  4.0K]  var/
    └── [drwxr-xr-x  4.0K]  lib/
        └── [drwxr-xr-x  4.0K]  sepolgen/
            └── [-rw-r--r--   33K]  perm_map

51 directories, 362 files
```
