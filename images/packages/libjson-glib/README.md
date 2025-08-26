# libjson-glib
```
└── [drwxr-xr-x  4.0K]  usr
    ├── [drwxr-xr-x  4.0K]  bin
    │   ├── [-rwxr-xr-x   54K]  gapplication
    │   ├── [-rwxr-xr-x  161K]  gdbus
    │   ├── [-rwxr-xr-x  2.0K]  gdbus-codegen
    │   ├── [-rwxr-xr-x  912K]  gi-compile-repository
    │   ├── [-rwxr-xr-x  125K]  gi-decompile-typelib
    │   ├── [-rwxr-xr-x   25K]  gi-inspect-typelib
    │   ├── [-rwxr-xr-x  313K]  gio
    │   ├── [-rwxr-xr-x   37K]  gio-querymodules
    │   ├── [-rwxr-xr-x  177K]  glib-compile-resources
    │   ├── [-rwxr-xr-x  216K]  glib-compile-schemas
    │   ├── [-rwxr-xr-x   40K]  glib-genmarshal
    │   ├── [-rwxr-xr-x  5.3K]  glib-gettextize
    │   ├── [-rwxr-xr-x   31K]  glib-mkenums
    │   ├── [-rwxr-xr-x   28K]  gobject-query
    │   ├── [-rwxr-xr-x   50K]  gresource
    │   ├── [-rwxr-xr-x   79K]  gsettings
    │   ├── [-rwxr-xr-x   74K]  gtester
    │   ├── [-rwxr-xr-x   19K]  gtester-report
    │   ├── [-rwxr-xr-x   38K]  json-glib-format
    │   ├── [-rwxr-xr-x   29K]  json-glib-validate
    │   └── [-rwxr-xr-x  2.0M]  pcre2grep
    ├── [drwxr-xr-x  4.0K]  include
    │   ├── [-rw-r--r--   13K]  ffi-x86_64.h
    │   ├── [-rw-r--r--   863]  ffi.h
    │   ├── [-rw-r--r--  4.2K]  ffitarget-x86_64.h
    │   ├── [-rw-r--r--   917]  ffitarget.h
    │   ├── [drwxr-xr-x  4.0K]  gio-unix-2.0
    │   │   └── [drwxr-xr-x  4.0K]  gio
    │   │       ├── [-rw-r--r--  8.5K]  gdesktopappinfo.h
    │   │       ├── [-rw-r--r--  2.2K]  gfiledescriptorbased.h
    │   │       ├── [-rw-r--r--  3.7K]  gunixfdmessage.h
    │   │       ├── [-rw-r--r--  2.9K]  gunixinputstream.h
    │   │       ├── [-rw-r--r--   12K]  gunixmounts.h
    │   │       └── [-rw-r--r--  2.9K]  gunixoutputstream.h
    │   ├── [drwxr-xr-x  4.0K]  glib-2.0
    │   │   ├── [drwxr-xr-x  4.0K]  gio
    │   │   │   ├── [-rw-r--r--  4.5K]  gaction.h
    │   │   │   ├── [-rw-r--r--  9.0K]  gactiongroup.h
    │   │   │   ├── [-rw-r--r--  1.8K]  gactiongroupexporter.h
    │   │   │   ├── [-rw-r--r--  4.3K]  gactionmap.h
    │   │   │   ├── [-rw-r--r--   20K]  gappinfo.h
    │   │   │   ├── [-rw-r--r--   15K]  gapplication.h
    │   │   │   ├── [-rw-r--r--  6.7K]  gapplicationcommandline.h
    │   │   │   ├── [-rw-r--r--  4.3K]  gasyncinitable.h
    │   │   │   ├── [-rw-r--r--  2.7K]  gasyncresult.h
    │   │   │   ├── [-rw-r--r--  5.0K]  gbufferedinputstream.h
    │   │   │   ├── [-rw-r--r--  3.2K]  gbufferedoutputstream.h
    │   │   │   ├── [-rw-r--r--  1.6K]  gbytesicon.h
    │   │   │   ├── [-rw-r--r--  3.9K]  gcancellable.h
    │   │   │   ├── [-rw-r--r--  2.5K]  gcharsetconverter.h
    │   │   │   ├── [-rw-r--r--  2.9K]  gcontenttype.h
    │   │   │   ├── [-rw-r--r--  3.0K]  gconverter.h
    │   │   │   ├── [-rw-r--r--  2.9K]  gconverterinputstream.h
    │   │   │   ├── [-rw-r--r--  2.9K]  gconverteroutputstream.h
    │   │   │   ├── [-rw-r--r--  3.4K]  gcredentials.h
    │   │   │   ├── [-rw-r--r--  6.4K]  gdatagrambased.h
    │   │   │   ├── [-rw-r--r--   11K]  gdatainputstream.h
    │   │   │   ├── [-rw-r--r--  4.7K]  gdataoutputstream.h
    │   │   │   ├── [-rw-r--r--  2.7K]  gdbusactiongroup.h
    │   │   │   ├── [-rw-r--r--  2.6K]  gdbusaddress.h
    │   │   │   ├── [-rw-r--r--  2.1K]  gdbusauthobserver.h
    │��  │   │   ├── [-rw-r--r--   41K]  gdbusconnection.h
    │   │   │   ├── [-rw-r--r--  4.3K]  gdbuserror.h
    │   │   │   ├── [-rw-r--r--  3.0K]  gdbusinterface.h
    │   │   │   ├── [-rw-r--r--  5.8K]  gdbusinterfaceskeleton.h
    │   │   │   ├── [-rw-r--r--   12K]  gdbusintrospection.h
    │   │   │   ├── [-rw-r--r--  1.7K]  gdbusmenumodel.h
    │   │   │   ├── [-rw-r--r--   11K]  gdbusmessage.h
    │   │   │   ├── [-rw-r--r--  6.8K]  gdbusmethodinvocation.h
    │   │   │   ├── [-rw-r--r--  4.8K]  gdbusnameowning.h
    │   │   │   ├── [-rw-r--r--  4.5K]  gdbusnamewatching.h
    │   │   │   ├── [-rw-r--r--  2.9K]  gdbusobject.h
    │   │   │   ├── [-rw-r--r--  4.4K]  gdbusobjectmanager.h
    │   │   │   ├── [-rw-r--r--  9.4K]  gdbusobjectmanagerclient.h
    │   │   │   ├── [-rw-r--r--  3.9K]  gdbusobjectmanagerserver.h
    │   │   │   ├── [-rw-r--r--  2.5K]  gdbusobjectproxy.h
    │   │   │   ├── [-rw-r--r--  3.7K]  gdbusobjectskeleton.h
    │   │   │   ├── [-rw-r--r--   12K]  gdbusproxy.h
    │   │   │   ├── [-rw-r--r--  2.5K]  gdbusserver.h
    │   │   │   ├── [-rw-r--r--  2.1K]  gdbusutils.h
    │   │   │   ├── [-rw-r--r--  2.5K]  gdebugcontroller.h
    │   │   │   ├── [-rw-r--r--  2.0K]  gdebugcontrollerdbus.h
    │   │   │   ├── [-rw-r--r--   14K]  gdrive.h
    │   │   │   ├── [-rw-r--r--  3.2K]  gdtlsclientconnection.h
    │   │   │   ├── [-rw-r--r--   12K]  gdtlsconnection.h
    │   │   │   ├── [-rw-r--r--  2.3K]  gdtlsserverconnection.h
    │   │   │   ├── [-rw-r--r--  2.1K]  gemblem.h
    │   │   │   ├── [-rw-r--r--  2.7K]  gemblemedicon.h
    │   │   │   ├── [-rw-r--r--   84K]  gfile.h
    │   │   │   ├── [-rw-r--r--  2.8K]  gfileattribute.h
    │   │   │   ├── [-rw-r--r--  6.2K]  gfileenumerator.h
    │   │   │   ├── [-rw-r--r--  1.9K]  gfileicon.h
    │   │   │   ├── [-rw-r--r--   52K]  gfileinfo.h
    │   │   │   ├── [-rw-r--r--  4.4K]  gfileinputstream.h
    │   │   │   ├── [-rw-r--r--  4.8K]  gfileiostream.h
    │   │   │   ├── [-rw-r--r--  3.2K]  gfilemonitor.h
    │   │   │   ├── [-rw-r--r--  3.0K]  gfilenamecompleter.h
    │   │   │   ├── [-rw-r--r--  5.1K]  gfileoutputstream.h
    │   │   │   ├── [-rw-r--r--  2.7K]  gfilterinputstream.h
    │   │   │   ├── [-rw-r--r--  2.7K]  gfilteroutputstream.h
    │   │   │   ├── [-rw-r--r--  4.2K]  gicon.h
    │   │   │   ├── [-rw-r--r--  5.1K]  ginetaddress.h
    │   │   │   ├── [-rw-r--r--  3.1K]  ginetaddressmask.h
    │   │   │   ├── [-rw-r--r--  3.1K]  ginetsocketaddress.h
    │   │   │   ├── [-rw-r--r--  2.9K]  ginitable.h
    │   │   │   ├── [-rw-r--r--  8.9K]  ginputstream.h
    │   │   │   ├── [-rw-r--r--  9.0K]  gio-autocleanups.h
    │   │   │   ├── [-rw-r--r--   48K]  gio-visibility.h
    │   │   │   ├── [-rw-r--r--  5.8K]  gio.h
    │   │   │   ├── [-rw-r--r--   83K]  gioenums.h
    │   │   │   ├── [-rw-r--r--   13K]  gioenumtypes.h
    │   │   │   ├── [-rw-r--r--  1.7K]  gioerror.h
    │   │   │   ├── [-rw-r--r--  8.0K]  giomodule.h
    │   │   │   ├── [-rw-r--r--  2.0K]  gioscheduler.h
    │   │   │   ├── [-rw-r--r--  4.7K]  giostream.h
    │   │   │   ├── [-rw-r--r--   23K]  giotypes.h
    │   │   │   ├── [-rw-r--r--  2.6K]  glistmodel.h
    │   │   │   ├── [-rw-r--r--  4.6K]  gliststore.h
    │   │   │   ├── [-rw-r--r--  3.5K]  gloadableicon.h
    │   │   │   ├── [-rw-r--r--  3.3K]  gmemoryinputstream.h
    │   │   │   ├── [-rw-r--r--  2.1K]  gmemorymonitor.h
    │   │   │   ├── [-rw-r--r--  3.8K]  gmemoryoutputstream.h
    │   │   │   ├── [-rw-r--r--  8.7K]  gmenu.h
    │   │   │   ├── [-rw-r--r--  1.9K]  gmenuexporter.h
    │   │   │   ├── [-rw-r--r--   14K]  gmenumodel.h
    │   │   │   ├── [-rw-r--r--   15K]  gmount.h
    │   │   │   ├── [-rw-r--r--  6.5K]  gmountoperation.h
    │   │   │   ├── [-rw-r--r--  2.5K]  gnativesocketaddress.h
    │   │   │   ├── [-rw-r--r--  2.3K]  gnativevolumemonitor.h
    │   │   │   ├── [-rw-r--r--  2.9K]  gnetworkaddress.h
    │   │   │   ├── [-rw-r--r--  2.0K]  gnetworking.h
    │   │   │   ├── [-rw-r--r--  4.2K]  gnetworkmonitor.h
    │   │   │   ├── [-rw-r--r--  2.7K]  gnetworkservice.h
    │   │   │   ├── [-rw-r--r--  5.0K]  gnotification.h
    │   │   │   ├── [-rw-r--r--   15K]  goutputstream.h
    │   │   │   ├── [-rw-r--r--  5.8K]  gpermission.h
    │   │   │   ├── [-rw-r--r--  3.7K]  gpollableinputstream.h
    │   │   │   ├── [-rw-r--r--  4.7K]  gpollableoutputstream.h
    │   │   │   ├── [-rw-r--r--  2.1K]  gpollableutils.h
    │   │   │   ├── [-rw-r--r--  2.3K]  gpowerprofilemonitor.h
    │   │   │   ├── [-rw-r--r--  2.0K]  gpropertyaction.h
    │   │   │   ├── [-rw-r--r--  3.9K]  gproxy.h
    │   │   │   ├── [-rw-r--r--  3.1K]  gproxyaddress.h
    │   │   │   ├── [-rw-r--r--  2.7K]  gproxyaddressenumerator.h
    │   │   │   ├── [-rw-r--r--  3.4K]  gproxyresolver.h
    │   │   │   ├── [-rw-r--r--  3.6K]  gremoteactiongroup.h
    │   │   │   ├── [-rw-r--r--   17K]  gresolver.h
    │   │   │   ├── [-rw-r--r--  4.8K]  gresource.h
    │   │   │   ├── [-rw-r--r--  3.2K]  gseekable.h
    │   │   │   ├── [-rw-r--r--   21K]  gsettings.h
    │   │   │   ├── [-rw-r--r--  8.3K]  gsettingsbackend.h
    │   │   │   ├── [-rw-r--r--  5.8K]  gsettingsschema.h
    │   │   │   ├── [-rw-r--r--  2.9K]  gsimpleaction.h
    │   │   │   ├── [-rw-r--r--  4.1K]  gsimpleactiongroup.h
    │   │   │   ├── [-rw-r--r--  7.6K]  gsimpleasyncresult.h
    │   │   │   ├── [-rw-r--r--  1.7K]  gsimpleiostream.h
    │   │   │   ├── [-rw-r--r--  1.7K]  gsimplepermission.h
    │   │   │   ├── [-rw-r--r--  3.4K]  gsimpleproxyresolver.h
    │   │   │   ├── [-rw-r--r--   17K]  gsocket.h
    │   │   │   ├── [-rw-r--r--  3.1K]  gsocketaddress.h
    │   │   │   ├── [-rw-r--r--  3.7K]  gsocketaddressenumerator.h
    │   │   │   ├── [-rw-r--r--   11K]  gsocketclient.h
    │   │   │   ├── [-rw-r--r--  2.8K]  gsocketconnectable.h
    │   │   │   ├── [-rw-r--r--  5.0K]  gsocketconnection.h
    │   │   │   ├── [-rw-r--r--  4.8K]  gsocketcontrolmessage.h
    │   │   │   ├── [-rw-r--r--  7.5K]  gsocketlistener.h
    │   │   │   ├── [-rw-r--r--  3.6K]  gsocketservice.h
    │   │   │   ├── [-rw-r--r--  1.9K]  gsrvtarget.h
    │   │   │   ├── [-rw-r--r--  8.4K]  gsubprocess.h
    │   │   │   ├── [-rw-r--r--  6.4K]  gsubprocesslauncher.h
    │   ���   │   ├── [-rw-r--r--  9.8K]  gtask.h
    │   │   │   ├── [-rw-r--r--  2.9K]  gtcpconnection.h
    │   │   │   ├── [-rw-r--r--  2.9K]  gtcpwrapperconnection.h
    │   │   │   ├── [-rw-r--r--  2.3K]  gtestdbus.h
    │   │   │   ├── [-rw-r--r--  2.5K]  gthemedicon.h
    │   │   │   ├── [-rw-r--r--  3.6K]  gthreadedsocketservice.h
    │   │   │   ├── [-rw-r--r--  4.5K]  gtlsbackend.h
    │   │   │   ├── [-rw-r--r--  5.1K]  gtlscertificate.h
    │   │   │   ├── [-rw-r--r--  3.6K]  gtlsclientconnection.h
    │   │   │   ├── [-rw-r--r--  8.4K]  gtlsconnection.h
    │   │   │   ├── [-rw-r--r--   17K]  gtlsdatabase.h
    │   │   │   ├── [-rw-r--r--  1.9K]  gtlsfiledatabase.h
    │   │   │   ├── [-rw-r--r--  8.2K]  gtlsinteraction.h
    │   │   │   ├── [-rw-r--r--  4.7K]  gtlspassword.h
    │   │   │   ├── [-rw-r--r--  2.2K]  gtlsserverconnection.h
    │   │   │   ├── [-rw-r--r--  5.7K]  gunixconnection.h
    │   │   │   ├── [-rw-r--r--  3.0K]  gunixcredentialsmessage.h
    │   │   │   ├── [-rw-r--r--  4.2K]  gunixfdlist.h
    │   │   │   ├── [-rw-r--r--  3.4K]  gunixsocketaddress.h
    │   │   │   ├── [-rw-r--r--  6.5K]  gvfs.h
    │   │   │   ├── [-rw-r--r--   11K]  gvolume.h
    │   │   │   ├── [-rw-r--r--  5.8K]  gvolumemonitor.h
    │   │   │   ├── [-rw-r--r--  2.6K]  gzlibcompressor.h
    │   │   │   └── [-rw-r--r--  2.2K]  gzlibdecompressor.h
    │   │   ├── [drwxr-xr-x  4.0K]  girepository
    │   │   │   ├── [-rw-r--r--   47K]  gi-visibility.h
    │   │   │   ├── [-rw-r--r--  3.2K]  giarginfo.h
    │   │   │   ├── [-rw-r--r--  4.0K]  gibaseinfo.h
    │   │   │   ├── [-rw-r--r--  5.0K]  gicallableinfo.h
    │   │   │   ├── [-rw-r--r--  1.9K]  gicallbackinfo.h
    │   │   │   ├── [-rw-r--r--  2.3K]  giconstantinfo.h
    │   │   │   ├── [-rw-r--r--  2.5K]  gienuminfo.h
    │   │   │   ├── [-rw-r--r--  2.7K]  gifieldinfo.h
    │   │   │   ├── [-rw-r--r--  1.8K]  giflagsinfo.h
    │   │   │   ├── [-rw-r--r--  3.7K]  gifunctioninfo.h
    │   │   │   ├── [-rw-r--r--  4.2K]  giinterfaceinfo.h
    │   │   │   ├── [-rw-r--r--  7.2K]  giobjectinfo.h
    │   │   │   ├── [-rw-r--r--  2.3K]  gipropertyinfo.h
    │   │   │   ├── [-rw-r--r--  2.5K]  giregisteredtypeinfo.h
    │   │   │   ├── [-rw-r--r--  2.6K]  girepository-autocleanups.h
    │   │   │   ├── [-rw-r--r--  9.5K]  girepository.h
    │   │   │   ├── [-rw-r--r--  4.7K]  girffi.h
    │   │   │   ├── [-rw-r--r--  2.2K]  gisignalinfo.h
    │   │   │   ├── [-rw-r--r--  3.2K]  gistructinfo.h
    │   │   │   ├── [-rw-r--r--  4.7K]  gitypeinfo.h
    │   │   │   ├── [-rw-r--r--  2.0K]  gitypelib.h
    │   │   │   ├── [-rw-r--r--   13K]  gitypes.h
    │   │   │   ├── [-rw-r--r--  3.5K]  giunioninfo.h
    │   │   │   ├── [-rw-r--r--  1.9K]  giunresolvedinfo.h
    │   │   │   ├── [-rw-r--r--  2.0K]  givalueinfo.h
    │   │   │   └── [-rw-r--r--  3.0K]  givfuncinfo.h
    │   │   ├── [drwxr-xr-x  4.0K]  glib
    │   │   │   ├── [drwxr-xr-x  4.0K]  deprecated
    │   │   │   │   ├── [-rw-r--r--  3.2K]  gallocator.h
    │   │   │   │   ├── [-rw-r--r--  3.0K]  gcache.h
    │   │   │   │   ├── [-rw-r--r--  2.9K]  gcompletion.h
    │   │   │   │   ├── [-rw-r--r--  4.3K]  gmain.h
    │   │   │   │   ├── [-rw-r--r--  2.9K]  grel.h
    │   │   │   │   └── [-rw-r--r--   11K]  gthread.h
    │   │   │   ├── [-rw-r--r--  5.3K]  galloca.h
    │   │   │   ├── [-rw-r--r--   14K]  garray.h
    │   │   │   ├── [-rw-r--r--  5.6K]  gasyncqueue.h
    │   │   │   ├── [-rw-r--r--   34K]  gatomic.h
    │   │   │   ├── [-rw-r--r--  2.8K]  gbacktrace.h
    │   │   │   ├── [-rw-r--r--  2.3K]  gbase64.h
    │   │   │   ├── [-rw-r--r--  4.6K]  gbitlock.h
    │   │   │   ├── [-rw-r--r--   15K]  gbookmarkfile.h
    │   │   │   ├── [-rw-r--r--  3.6K]  gbytes.h
    │   │   │   ├── [-rw-r--r--  1.6K]  gcharset.h
    │   │   │   ├── [-rw-r--r--  3.6K]  gchecksum.h
    │   │   │   ├── [-rw-r--r--  5.8K]  gconvert.h
    │   │   │   ├── [-rw-r--r--  6.4K]  gdataset.h
    │   │   │   ├── [-rw-r--r--   12K]  gdate.h
    │   │   │   ├── [-rw-r--r--   14K]  gdatetime.h
    │   │   │   ├── [-rw-r--r--  1.8K]  gdir.h
    │   │   │   ├── [-rw-r--r--  2.4K]  genviron.h
    │   │   │   ├── [-rw-r--r--   11K]  gerror.h
    │   │   │   ├── [-rw-r--r--  7.7K]  gfileutils.h
    │   │   │   ├── [-rw-r--r--  2.4K]  ggettext.h
    │   │   │   ├── [-rw-r--r--  8.4K]  ghash.h
    │   │   │   ├── [-rw-r--r--  3.3K]  ghmac.h
    │   │   │   ├── [-rw-r--r--  6.3K]  ghook.h
    │   │   │   ├── [-rw-r--r--  1.5K]  ghostutils.h
    │   │   │   ├── [-rw-r--r--  1.4K]  gi18n-lib.h
    │   │   │   ├── [-rw-r--r--  1.2K]  gi18n.h
    │   │   │   ├── [-rw-r--r--   14K]  giochannel.h
    │   │   │   ├── [-rw-r--r--   15K]  gkeyfile.h
    │   │   │   ├── [-rw-r--r--  5.0K]  glib-autocleanups.h
    │   │   │   ├── [-rw-r--r--  1.7K]  glib-typeof.h
    │   │   │   ├── [-rw-r--r--   49K]  glib-visibility.h
    │   │   │   ├── [-rw-r--r--  6.8K]  glist.h
    │   │   │   ├── [-rw-r--r--   51K]  gmacros.h
    │   │   │   ├── [-rw-r--r--   36K]  gmain.h
    │   │   │   ├── [-rw-r--r--  2.0K]  gmappedfile.h
    │   │   │   ├── [-rw-r--r--   11K]  gmarkup.h
    │   │   │   ├── [-rw-r--r--   16K]  gmem.h
    │   │   │   ├── [-rw-r--r--   27K]  gmessages.h
    │   │   │   ├── [-rw-r--r--  8.5K]  gnode.h
    │   │   │   ├── [-rw-r--r--   17K]  goption.h
    │   │   │   ├── [-rw-r--r--  2.4K]  gpathbuf.h
    │   │   │   ├── [-rw-r--r--  2.3K]  gpattern.h
    │   │   │   ├── [-rw-r--r--  4.1K]  gpoll.h
    │   │   │   ├── [-rw-r--r--  1.7K]  gprimes.h
    │   │   │   ├── [-rw-r--r--  2.0K]  gprintf.h
    │   │   │   ├── [-rw-r--r--  1.8K]  gqsort.h
    │   │   │   ├── [-rw-r--r--  2.7K]  gquark.h
    │   │   │   ├── [-rw-r--r--  7.6K]  gqueue.h
    │   │   │   ├── [-rw-r--r--  3.2K]  grand.h
    │   │   │   ├── [-rw-r--r--  3.8K]  grcbox.h
    │   │   │   ├── [-rw-r--r--  5.1K]  grefcount.h
    │   │   │   ├── [-rw-r--r--  2.0K]  grefstring.h
    │   │   │   ├── [-rw-r--r--   28K]  gregex.h
    │   │   │   ├── [-rw-r--r--  8.7K]  gscanner.h
    │   │   │   ├── [-rw-r--r--  8.7K]  gsequence.h
    │   │   │   ├── [-rw-r--r--  1.8K]  gshell.h
    │   │   │   ├── [-rw-r--r--  4.6K]  gslice.h
    │   │   │   ├── [-rw-r--r--  6.4K]  gslist.h
    │   │   │   ├── [-rw-r--r--   14K]  gspawn.h
    │   │   │   ├── [-rw-r--r--  8.0K]  gstdio.h
    │   │   │   ├── [-rw-r--r--   19K]  gstrfuncs.h
    │   │   │   ├── [-rw-r--r--   12K]  gstring.h
    │   │   │   ├── [-rw-r--r--  2.1K]  gstringchunk.h
    │   │   │   ├── [-rw-r--r--  2.0K]  gstrvbuilder.h
    │   │   │   ├── [-rw-r--r--   41K]  gtestutils.h
    │   │   │   ├── [-rw-r--r--   26K]  gthread.h
    │   │   │   ├── [-rw-r--r--  4.3K]  gthreadpool.h
    │   │   │   ├── [-rw-r--r--  2.6K]  gtimer.h
    │   │   │   ├── [-rw-r--r--  3.9K]  gtimezone.h
    │   │   │   ├── [-rw-r--r--  1.9K]  gtrashstack.h
    │   │   │   ├── [-rw-r--r--  6.4K]  gtree.h
    │   │   │   ├── [-rw-r--r--   20K]  gtypes.h
    │   │   │   ├── [-rw-r--r--   43K]  gunicode.h
    │   │   │   ├── [-rw-r--r--   16K]  guri.h
    │   │   │   ├── [-rw-r--r--   14K]  gutils.h
    │   │   │   ├── [-rw-r--r--  1.3K]  guuid.h
    │   │   │   ├── [-rw-r--r--   31K]  gvariant.h
    │   │   │   ├── [-rw-r--r--   13K]  gvarianttype.h
    │   │   │   ├── [-rw-r--r--  2.0K]  gversion.h
    │   │   │   └── [-rw-r--r--   14K]  gversionmacros.h
    │   │   ├── [-rw-r--r--  1.5K]  glib-object.h
    │   │   ├── [-rw-r--r--   11K]  glib-unix.h
    │   │   ├── [-rw-r--r--  3.4K]  glib.h
    │   │   ├── [drwxr-xr-x  4.0K]  gmodule
    │   │   │   └── [-rw-r--r--   52K]  gmodule-visibility.h
    │   │   ├── [-rw-r--r--  5.2K]  gmodule.h
    │   │   └── [drwxr-xr-x  4.0K]  gobject
    │   │       ├── [-rw-r--r--  6.4K]  gbinding.h
    │   │       ├── [-rw-r--r--  3.8K]  gbindinggroup.h
    │   │       ├── [-rw-r--r--  4.0K]  gboxed.h
    │   │       ├── [-rw-r--r--   11K]  gclosure.h
    │   │       ├── [-rw-r--r--   11K]  genums.h
    │   │       ├── [-rw-r--r--  1007]  glib-enumtypes.h
    │   │       ├── [-rw-r--r--   10K]  glib-types.h
    │   │       ├── [-rw-r--r--   21K]  gmarshal.h
    │   │       ├── [-rw-r--r--  1.4K]  gobject-autocleanups.h
    │   │       ├── [-rw-r--r--   52K]  gobject-visibility.h
    │   │       ├── [-rw-r--r--   34K]  gobject.h
    │   │       ├── [-rw-r--r--  5.5K]  gobjectnotifyqueue.c
    │   │       ├── [-rw-r--r--   17K]  gparam.h
    │   │       ├── [-rw-r--r--   33K]  gparamspecs.h
    │   │       ├── [-rw-r--r--   27K]  gsignal.h
    │   │       ├── [-rw-r--r--  4.2K]  gsignalgroup.h
    │   │       ├── [-rw-r--r--  1.3K]  gsourceclosure.h
    │   │       ├── [-rw-r--r--  101K]  gtype.h
    │   │       ├── [-rw-r--r--   11K]  gtypemodule.h
    │   │       ├── [-rw-r--r--  4.8K]  gtypeplugin.h
    │   │       ├── [-rw-r--r--  7.5K]  gvalue.h
    │   │       ├── [-rw-r--r--  3.1K]  gvaluearray.h
    │   │       ├── [-rw-r--r--   10K]  gvaluecollector.h
    │   │       └── [-rw-r--r--   10K]  gvaluetypes.h
    │   ├── [drwxr-xr-x  4.0K]  json-glib-1.0
    │   │   └── [drwxr-xr-x  4.0K]  json-glib
    │   │       ├── [-rw-r--r--  4.0K]  json-builder.h
    │   │       ├── [-rw-r--r--  1005]  json-enum-types.h
    │   │       ├── [-rw-r--r--  4.7K]  json-generator.h
    │   │       ├── [-rw-r--r--  1.4K]  json-glib.h
    │   │       ├── [-rw-r--r--   11K]  json-gobject.h
    │   │       ├── [-rw-r--r--  1.9K]  json-gvariant.h
    │   │       ├── [-rw-r--r--  8.6K]  json-parser.h
    │   │       ├── [-rw-r--r--  2.6K]  json-path.h
    │   │       ├── [-rw-r--r--  5.7K]  json-reader.h
    │   │       ├── [-rw-r--r--   23K]  json-types.h
    │   │       ├── [-rw-r--r--  1.3K]  json-utils.h
    │   │       ├── [-rw-r--r--  8.1K]  json-version-macros.h
    │   │       └── [-rw-r--r--  3.0K]  json-version.h
    │   ├── [-rw-r--r--   47K]  pcre2.h
    │   ├── [-rw-r--r--  7.2K]  pcre2posix.h
    │   ├── [-rw-r--r--   16K]  zconf.h
    │   └── [-rw-r--r--   95K]  zlib.h
    ├── [drwxr-xr-x  4.0K]  lib64
    │   ├── [drwxr-xr-x  4.0K]  gio
    │   │   └── [drwxr-xr-x  4.0K]  modules
    │   ├── [drwxr-xr-x  4.0K]  glib-2.0
    │   │   └── [drwxr-xr-x  4.0K]  include
    │   │       └── [-rw-r--r--  5.9K]  glibconfig.h
    │   ├── [lrwxrwxrwx    11]  libffi.so -> libffi.so.7
    │   ├── [lrwxrwxrwx    15]  libffi.so.7 -> libffi.so.7.1.0
    │   ├── [-rwxr-xr-x  158K]  libffi.so.7.1.0
    │   ├── [lrwxrwxrwx    15]  libgio-2.0.so -> libgio-2.0.so.0
    │   ├── [lrwxrwxrwx    22]  libgio-2.0.so.0 -> libgio-2.0.so.0.8504.0
    │   ├── [-rwxr-xr-x  9.1M]  libgio-2.0.so.0.8504.0
    │   ├── [lrwxrwxrwx    24]  libgirepository-2.0.so -> libgirepository-2.0.so.0
    │   ├── [lrwxrwxrwx    31]  libgirepository-2.0.so.0 ->                           ↵
libgirepository-2.0.so.0.8504.0
    │   ├── [-rwxr-xr-x  1.1M]  libgirepository-2.0.so.0.8504.0
    │   ├── [lrwxrwxrwx    16]  libglib-2.0.so -> libglib-2.0.so.0
    │   ├── [lrwxrwxrwx    23]  libglib-2.0.so.0 -> libglib-2.0.so.0.8504.0
    │   ├── [-rwxr-xr-x  6.8M]  libglib-2.0.so.0.8504.0
    │   ├── [lrwxrwxrwx    19]  libgmodule-2.0.so -> libgmodule-2.0.so.0
    │   ├── [lrwxrwxrwx    26]  libgmodule-2.0.so.0 ->                         ↵

    │   ├── [-rwxr-xr-x   58K]  libgmodule-2.0.so.0.8504.0
    │   ├── [lrwxrwxrwx    19]  libgobject-2.0.so -> libgobject-2.0.so.0
    │   ├── [lrwxrwxrwx    26]  libgobject-2.0.so.0 -> libgobject-2.0.so.0.8504.0
    │   ├── [-rwxr-xr-x  1.6M]  libgobject-2.0.so.0.8504.0
    │   ├── [lrwxrwxrwx    19]  libgthread-2.0.so -> libgthread-2.0.so.0
    │   ├── [lrwxrwxrwx    26]  libgthread-2.0.so.0 -> libgthread-2.0.so.0.8504.0
    │   ├── [-rwxr-xr-x   18K]  libgthread-2.0.so.0.8504.0
    │   ├── [lrwxrwxrwx    21]  libjson-glib-1.0.so -> libjson-glib-1.0.so.0
    │   ├── [lrwxrwxrwx    28]  libjson-glib-1.0.so.0 ->                       ↵

    │   ├── [-rwxr-xr-x  696K]  libjson-glib-1.0.so.0.1000.6
    │   ├── [-rw-r--r--  2.5M]  libpcre2-16.a
    │   ├── [-rw-r--r--  2.4M]  libpcre2-32.a
    │   ├── [-rw-r--r--  2.6M]  libpcre2-8.a
    │   ├── [-rw-r--r--   18K]  libpcre2-posix.a
    │   ├── [lrwxrwxrwx     9]  libz.so -> libz.so.1
    │   ├── [lrwxrwxrwx    13]  libz.so.1 -> libz.so.1.3.1
    │   ├── [-rwxr-xr-x  294K]  libz.so.1.3.1
    │   └── [drwxr-xr-x  4.0K]  pkgconfig
    │       ├── [-rw-r--r--   698]  gio-2.0.pc
    │       ├── [-rw-r--r--   215]  gio-unix-2.0.pc
    │       ├── [-rw-r--r--   536]  girepository-2.0.pc
    │       ├── [-rw-r--r--   511]  glib-2.0.pc
    │       ├── [-rw-r--r--   260]  gmodule-2.0.pc
    │       ├── [-rw-r--r--   260]  gmodule-export-2.0.pc
    │       ├── [-rw-r--r--   260]  gmodule-no-export-2.0.pc
    │       ├── [-rw-r--r--   260]  gobject-2.0.pc
    │       ├── [-rw-r--r--   229]  gthread-2.0.pc
    │       ├── [-rw-r--r--   289]  json-glib-1.0.pc
    │       ├── [-rw-r--r--   205]  libffi.pc
    │       ├── [-rw-r--r--   280]  libpcre2-16.pc
    │       ├── [-rw-r--r--   280]  libpcre2-32.pc
    │       ├── [-rw-r--r--   277]  libpcre2-8.pc
    │       └── [-rw-r--r--   179]  zlib.pc
    └── [drwxr-xr-x  4.0K]  libexec
        ├── [-rwxr-xr-x   30K]  gio-launch-desktop
        └── [drwxr-xr-x  4.0K]  installed-tests
            └── [drwxr-xr-x  4.0K]  json-glib-1.0
                ├── [-rwxr-xr-x   40K]  array
                ├── [-rwxr-xr-x   42K]  boxed
                ├── [-rwxr-xr-x   30K]  builder
                ├── [-rwxr-xr-x   57K]  generator
                ├── [-rwxr-xr-x   35K]  gvariant
                ├── [-rwxr-xr-x   38K]  invalid
                ├── [-rw-r--r--     5]  invalid.json
                ├── [-rwxr-xr-x   73K]  node
                ├── [-rwxr-xr-x   59K]  object
                ├── [-rwxr-xr-x   95K]  parser
                ├── [-rwxr-xr-x   38K]  path
                ├── [-rwxr-xr-x   59K]  reader
                ├── [-rwxr-xr-x   49K]  serialize-complex
                ├── [-rwxr-xr-x   56K]  serialize-full
                ├── [-rwxr-xr-x   34K]  serialize-simple
                ├── [-rw-r--r--    44]  skip-bom.json
                └── [-rw-r--r--    29]  stream-load.json

24 directories, 412 files
```