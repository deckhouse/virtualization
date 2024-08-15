package route

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target amd64 -type route_event ebpf ../../../bpf/route_watcher.c
