package api

type Operation interface {
	VirtualMachine()
	Console()
}
