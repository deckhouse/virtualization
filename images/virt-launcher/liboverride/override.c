#define _GNU_SOURCE 1
#include <stdio.h>
#include <errno.h>
#include <dlfcn.h>

// Override getpeercon to return ENOPROTOOPT instead of EINVAL.
int getpeercon(int fd, char ** context)
{
  // Get pointer to original getpeercon.
  int (*getpeercon_orig)(int fd, char ** context);
  getpeercon_orig = dlsym(RTLD_NEXT, "getpeercon");
  // Run original getpeercon.
  int ret = (*getpeercon_orig)(fd, context);
  if (ret < 0) {
    if (errno == EINVAL) {
      fprintf(stderr,"getpeercon overridden to return %d\n", ENOPROTOOPT);
      errno = ENOPROTOOPT;
    }
  }
  return ret;
}
