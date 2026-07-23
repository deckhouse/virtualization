FROM busybox:1.36.1-glibc

# The binary and dlv are cross-built on the host in go.work workspace mode by
# `task dlv:virtualization-dra-usb:build`; the Dockerfile only packages them.
WORKDIR /app
COPY ./images/virtualization-dra-usb/debug/out/dlv /app/dlv
COPY ./images/virtualization-dra-usb/debug/out/virtualization-dra-usb /app/virtualization-dra-usb
USER 65532:65532

ENTRYPOINT ["./dlv", "--listen=:2345", "--headless=true", "--continue", "--log=true", "--log-output=debugger,debuglineerr,gdbwire,lldbout,rpc", "--accept-multiclient", "--api-version=2", "exec", "./virtualization-dra-usb", "--"]
