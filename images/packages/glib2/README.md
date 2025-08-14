# glib2
```
/out
└── usr
    ├── bin
    │   ├── gapplication
    │   ├── gdbus
    │   ├── gdbus-codegen
    │   ├── gi-compile-repository
    │   ├── gi-decompile-typelib
    │   ├── gi-inspect-typelib
    │   ├── gio
    │   ├── gio-querymodules
    │   ├── glib-compile-resources
    │   ├── glib-compile-schemas
    │   ├── glib-genmarshal
    │   ├── glib-gettextize
    │   ├── glib-mkenums
    │   ├── gobject-query
    │   ├── gresource
    │   ├── gsettings
    │   ├── gtester
    │   └── gtester-report
    ├── include
    │   ├── gio-unix-2.0
    │   │   └── gio
    │   │       ├── gdesktopappinfo.h
    │   │       ├── gfiledescriptorbased.h
    │   │       ├── gunixfdmessage.h
    │   │       ├── gunixinputstream.h
    │   │       ├── gunixmounts.h
    │   │       └── gunixoutputstream.h
    │   └── glib-2.0
    │       ├── gio
    │       │   ├── gaction.h
    │       │   ├── gactiongroup.h
    │       │   ├── gactiongroupexporter.h
    │       │   ├── gactionmap.h
    │       │   ├── gappinfo.h
    │       │   ├── gapplication.h
    │       │   ├── gapplicationcommandline.h
    │       │   ├── gasyncinitable.h
    │       │   ├── gasyncresult.h
    │       │   ├── gbufferedinputstream.h
    │       │   ├── gbufferedoutputstream.h
    │       │   ├── gbytesicon.h
    │       │   ├── gcancellable.h
    │       │   ├── gcharsetconverter.h
    │       │   ├── gcontenttype.h
    │       │   ├── gconverter.h
    │       │   ├── gconverterinputstream.h
    │       │   ├── gconverteroutputstream.h
    │       │   ├── gcredentials.h
    │       │   ├── gdatagrambased.h
    │       │   ├── gdatainputstream.h
    │       │   ├── gdataoutputstream.h
    │       │   ├── gdbusactiongroup.h
    │       │   ├── gdbusaddress.h
    │       │   ├── gdbusauthobserver.h
    │       │   ├── gdbusconnection.h
    │       │   ├── gdbuserror.h
    │       │   ├── gdbusinterface.h
    │       │   ├── gdbusinterfaceskeleton.h
    │       │   ├── gdbusintrospection.h
    │       │   ├── gdbusmenumodel.h
    │       │   ├── gdbusmessage.h
    │       │   ├── gdbusmethodinvocation.h
    │       │   ├── gdbusnameowning.h
    │       │   ├── gdbusnamewatching.h
    │       │   ├── gdbusobject.h
    │       │   ├── gdbusobjectmanager.h
    │       │   ├── gdbusobjectmanagerclient.h
    │       │   ├── gdbusobjectmanagerserver.h
    │       │   ├── gdbusobjectproxy.h
    │       │   ├── gdbusobjectskeleton.h
    │       │   ├── gdbusproxy.h
    │       │   ├── gdbusserver.h
    │       │   ├── gdbusutils.h
    │       │   ├── gdebugcontroller.h
    │       │   ├── gdebugcontrollerdbus.h
    │       │   ├── gdrive.h
    │       │   ├── gdtlsclientconnection.h
    │       │   ├── gdtlsconnection.h
    │       │   ├── gdtlsserverconnection.h
    │       │   ├── gemblem.h
    │       │   ├── gemblemedicon.h
    │       │   ├── gfile.h
    │       │   ├── gfileattribute.h
    │       │   ├── gfileenumerator.h
    │       │   ├── gfileicon.h
    │       │   ├── gfileinfo.h
    │       │   ├── gfileinputstream.h
    │       │   ├── gfileiostream.h
    │       │   ├── gfilemonitor.h
    │       │   ├── gfilenamecompleter.h
    │       │   ├── gfileoutputstream.h
    │       │   ├── gfilterinputstream.h
    │       │   ├── gfilteroutputstream.h
    │       │   ├── gicon.h
    │       │   ├── ginetaddress.h
    │       │   ├── ginetaddressmask.h
    │       │   ├── ginetsocketaddress.h
    │       │   ├── ginitable.h
    │       │   ├── ginputstream.h
    │       │   ├── gio-autocleanups.h
    │       │   ├── gio-visibility.h
    │       │   ├── gio.h
    │       │   ├── gioenums.h
    │       │   ├── gioenumtypes.h
    │       │   ├── gioerror.h
    │       │   ├── giomodule.h
    │       │   ├── gioscheduler.h
    │       │   ├── giostream.h
    │       │   ├── giotypes.h
    │       │   ├── glistmodel.h
    │       │   ├── gliststore.h
    │       │   ├── gloadableicon.h
    │       │   ├── gmemoryinputstream.h
    │       │   ├── gmemorymonitor.h
    │       │   ├── gmemoryoutputstream.h
    │       │   ├── gmenu.h
    │       │   ├── gmenuexporter.h
    │       │   ├── gmenumodel.h
    │       │   ├── gmount.h
    │       │   ├── gmountoperation.h
    │       │   ├── gnativesocketaddress.h
    │       │   ├── gnativevolumemonitor.h
    │       │   ├── gnetworkaddress.h
    │       │   ├── gnetworking.h
    │       │   ├── gnetworkmonitor.h
    │       │   ├── gnetworkservice.h
    │       │   ├── gnotification.h
    │       │   ├── goutputstream.h
    │       │   ├── gpermission.h
    │       │   ├── gpollableinputstream.h
    │       │   ├── gpollableoutputstream.h
    │       │   ├── gpollableutils.h
    │       │   ├── gpowerprofilemonitor.h
    │       │   ├── gpropertyaction.h
    │       │   ├── gproxy.h
    │       │   ├── gproxyaddress.h
    │       │   ├── gproxyaddressenumerator.h
    │       │   ├── gproxyresolver.h
    │       │   ├── gremoteactiongroup.h
    │       │   ├── gresolver.h
    │       │   ├── gresource.h
    │       │   ├── gseekable.h
    │       │   ├── gsettings.h
    │       │   ├── gsettingsbackend.h
    │       │   ├── gsettingsschema.h
    │       │   ├── gsimpleaction.h
    │       │   ├── gsimpleactiongroup.h
    │       │   ├── gsimpleasyncresult.h
    │       │   ├── gsimpleiostream.h
    │       │   ├── gsimplepermission.h
    │       │   ├── gsimpleproxyresolver.h
    │       │   ├── gsocket.h
    │       │   ├── gsocketaddress.h
    │       │   ├── gsocketaddressenumerator.h
    │       │   ├── gsocketclient.h
    │       │   ├── gsocketconnectable.h
    │       │   ├── gsocketconnection.h
    │       │   ├── gsocketcontrolmessage.h
    │       │   ├── gsocketlistener.h
    │       │   ├── gsocketservice.h
    │       │   ├── gsrvtarget.h
    │       │   ├── gsubprocess.h
    │       │   ├── gsubprocesslauncher.h
    │       │   ├── gtask.h
    │       │   ├── gtcpconnection.h
    │       │   ├── gtcpwrapperconnection.h
    │       │   ├── gtestdbus.h
    │       │   ├── gthemedicon.h
    │       │   ├── gthreadedsocketservice.h
    │       │   ├── gtlsbackend.h
    │       │   ├── gtlscertificate.h
    │       │   ├── gtlsclientconnection.h
    │       │   ├── gtlsconnection.h
    │       │   ├── gtlsdatabase.h
    │       │   ├── gtlsfiledatabase.h
    │       │   ├── gtlsinteraction.h
    │       │   ├── gtlspassword.h
    │       │   ├── gtlsserverconnection.h
    │       │   ├── gunixconnection.h
    │       │   ├── gunixcredentialsmessage.h
    │       │   ├── gunixfdlist.h
    │       │   ├── gunixsocketaddress.h
    │       │   ├── gvfs.h
    │       │   ├── gvolume.h
    │       │   ├── gvolumemonitor.h
    │       │   ├── gzlibcompressor.h
    │       │   └── gzlibdecompressor.h
    │       ├── girepository
    │       │   ├── gi-visibility.h
    │       │   ├── giarginfo.h
    │       │   ├── gibaseinfo.h
    │       │   ├── gicallableinfo.h
    │       │   ├── gicallbackinfo.h
    │       │   ├── giconstantinfo.h
    │       │   ├── gienuminfo.h
    │       │   ├── gifieldinfo.h
    │       │   ├── giflagsinfo.h
    │       │   ├── gifunctioninfo.h
    │       │   ├── giinterfaceinfo.h
    │       │   ├── giobjectinfo.h
    │       │   ├── gipropertyinfo.h
    │       │   ├── giregisteredtypeinfo.h
    │       │   ├── girepository-autocleanups.h
    │       │   ├── girepository.h
    │       │   ├── girffi.h
    │       │   ├── gisignalinfo.h
    │       │   ├── gistructinfo.h
    │       │   ├── gitypeinfo.h
    │       │   ├── gitypelib.h
    │       │   ├── gitypes.h
    │       │   ├── giunioninfo.h
    │       │   ├── giunresolvedinfo.h
    │       │   ├── givalueinfo.h
    │       │   └── givfuncinfo.h
    │       ├── glib
    │       │   ├── deprecated
    │       │   │   ├── gallocator.h
    │       │   │   ├── gcache.h
    │       │   │   ├── gcompletion.h
    │       │   │   ├── gmain.h
    │       │   │   ├── grel.h
    │       │   │   └── gthread.h
    │       │   ├── galloca.h
    │       │   ├── garray.h
    │       │   ├── gasyncqueue.h
    │       │   ├── gatomic.h
    │       │   ├── gbacktrace.h
    │       │   ├── gbase64.h
    │       │   ├── gbitlock.h
    │       │   ├── gbookmarkfile.h
    │       │   ├── gbytes.h
    │       │   ├── gcharset.h
    │       │   ├── gchecksum.h
    │       │   ├── gconvert.h
    │       │   ├── gdataset.h
    │       │   ├── gdate.h
    │       │   ├── gdatetime.h
    │       │   ├── gdir.h
    │       │   ├── genviron.h
    │       │   ├── gerror.h
    │       │   ├── gfileutils.h
    │       │   ├── ggettext.h
    │       │   ├── ghash.h
    │       │   ├── ghmac.h
    │       │   ├── ghook.h
    │       │   ├── ghostutils.h
    │       │   ├── gi18n-lib.h
    │       │   ├── gi18n.h
    │       │   ├── giochannel.h
    │       │   ├── gkeyfile.h
    │       │   ├── glib-autocleanups.h
    │       │   ├── glib-typeof.h
    │       │   ├── glib-visibility.h
    │       │   ├── glist.h
    │       │   ├── gmacros.h
    │       │   ├── gmain.h
    │       │   ├── gmappedfile.h
    │       │   ├── gmarkup.h
    │       │   ├── gmem.h
    │       │   ├── gmessages.h
    │       │   ├── gnode.h
    │       │   ├── goption.h
    │       │   ├── gpathbuf.h
    │       │   ├── gpattern.h
    │       │   ├── gpoll.h
    │       │   ├── gprimes.h
    │       │   ├── gprintf.h
    │       │   ├── gqsort.h
    │       │   ├── gquark.h
    │       │   ├── gqueue.h
    │       │   ├── grand.h
    │       │   ├── grcbox.h
    │       │   ├── grefcount.h
    │       │   ├── grefstring.h
    │       │   ├── gregex.h
    │       │   ├── gscanner.h
    │       │   ├── gsequence.h
    │       │   ├── gshell.h
    │       │   ├── gslice.h
    │       │   ├── gslist.h
    │       │   ├── gspawn.h
    │       │   ├── gstdio.h
    │       │   ├── gstrfuncs.h
    │       │   ├── gstring.h
    │       │   ├── gstringchunk.h
    │       │   ├── gstrvbuilder.h
    │       │   ├── gtestutils.h
    │       │   ├── gthread.h
    │       │   ├── gthreadpool.h
    │       │   ├── gtimer.h
    │       │   ├── gtimezone.h
    │       │   ├── gtrashstack.h
    │       │   ├── gtree.h
    │       │   ├── gtypes.h
    │       │   ├── gunicode.h
    │       │   ├── guri.h
    │       │   ├── gutils.h
    │       │   ├── guuid.h
    │       │   ├── gvariant.h
    │       │   ├── gvarianttype.h
    │       │   ├── gversion.h
    │       │   └── gversionmacros.h
    │       ├── glib-object.h
    │       ├── glib-unix.h
    │       ├── glib.h
    │       ├── gmodule
    │       │   └── gmodule-visibility.h
    │       ├── gmodule.h
    │       └── gobject
    │           ├── gbinding.h
    │           ├── gbindinggroup.h
    │           ├── gboxed.h
    │           ├── gclosure.h
    │           ├── genums.h
    │           ├── glib-enumtypes.h
    │           ├── glib-types.h
    │           ├── gmarshal.h
    │           ├── gobject-autocleanups.h
    │           ├── gobject-visibility.h
    │           ├── gobject.h
    │           ├── gobjectnotifyqueue.c
    │           ├── gparam.h
    │           ├── gparamspecs.h
    │           ├── gsignal.h
    │           ├── gsignalgroup.h
    │           ├── gsourceclosure.h
    │           ├── gtype.h
    │           ├── gtypemodule.h
    │           ├── gtypeplugin.h
    │           ├── gvalue.h
    │           ├── gvaluearray.h
    │           ├── gvaluecollector.h
    │           └── gvaluetypes.h
    ├── lib64
    │   ├── gio
    │   │   └── modules
    │   ├── glib-2.0
    │   │   └── include
    │   │       └── glibconfig.h
    │   ├── libgio-2.0.so -> libgio-2.0.so.0
    │   ├── libgio-2.0.so.0 -> libgio-2.0.so.0.8200.5
    │   ├── libgio-2.0.so.0.8200.5
    │   ├── libgirepository-2.0.so -> libgirepository-2.0.so.0
    │   ├── libgirepository-2.0.so.0 -> libgirepository-2.0.so.0.8200.5
    │   ├── libgirepository-2.0.so.0.8200.5
    │   ├── libglib-2.0.so -> libglib-2.0.so.0
    │   ├── libglib-2.0.so.0 -> libglib-2.0.so.0.8200.5
    │   ├── libglib-2.0.so.0.8200.5
    │   ├── libgmodule-2.0.so -> libgmodule-2.0.so.0
    │   ├── libgmodule-2.0.so.0 -> libgmodule-2.0.so.0.8200.5
    │   ├── libgmodule-2.0.so.0.8200.5
    │   ├── libgobject-2.0.so -> libgobject-2.0.so.0
    │   ├── libgobject-2.0.so.0 -> libgobject-2.0.so.0.8200.5
    │   ├── libgobject-2.0.so.0.8200.5
    │   ├── libgthread-2.0.so -> libgthread-2.0.so.0
    │   ├── libgthread-2.0.so.0 -> libgthread-2.0.so.0.8200.5
    │   ├── libgthread-2.0.so.0.8200.5
    │   └── pkgconfig
    │       ├── gio-2.0.pc
    │       ├── gio-unix-2.0.pc
    │       ├── girepository-2.0.pc
    │       ├── glib-2.0.pc
    │       ├── gmodule-2.0.pc
    │       ├── gmodule-export-2.0.pc
    │       ├── gmodule-no-export-2.0.pc
    │       ├── gobject-2.0.pc
    │       └── gthread-2.0.pc
    ├── libexec
    │   └── gio-launch-desktop
    └── share
        ├── aclocal
        │   ├── glib-2.0.m4
        │   ├── glib-gettext.m4
        │   └── gsettings.m4
        ├── bash-completion
        │   └── completions
        │       ├── gapplication
        │       ├── gdbus
        │       ├── gio
        │       ├── gresource
        │       └── gsettings
        ├── gdb
        │   └── auto-load
        │       └── out
        │           └── usr
        │               └── lib64
        │                   ├── libglib-2.0.so.0.8200.5-gdb.py
        │                   └── libgobject-2.0.so.0.8200.5-gdb.py
        ├── gettext
        │   └── its
        │       ├── gschema.its
        │       └── gschema.loc
        └── glib-2.0
            ├── codegen
            │   ├── __init__.py
            │   ├── codegen.py
            │   ├── codegen_docbook.py
            │   ├── codegen_main.py
            │   ├── codegen_md.py
            │   ├── codegen_rst.py
            │   ├── config.py
            │   ├── dbustypes.py
            │   ├── parser.py
            │   └── utils.py
            ├── dtds
            │   └── gresource.dtd
            ├── gdb
            │   ├── glib_gdb.py
            │   └── gobject_gdb.py
            ├── schemas
            │   └── gschema.dtd
            └── valgrind
                └── glib.supp

37 directories, 379 files
```