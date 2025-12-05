# 001-util-ulockmgr_server-c-conditionally-define-closefrom-fix-glibc-2-34.patch
closefrom(3) has joined us in glibc-land from *BSD and Solaris. Since
it's available in glibc 2.34+, we want to detect it and only define our
fallback if the libc doesn't provide it.
https://github.com/libfuse/libfuse/commit/5a43d0f724c56f8836f3f92411e0de1b5f82db32]
https://bugs.gentoo.org/803923