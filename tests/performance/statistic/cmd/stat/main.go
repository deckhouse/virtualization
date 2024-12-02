package main

import (
	"flag"

	"statistic/internal/helpers"
	"statistic/internal/vd"
	"statistic/internal/vm"
)

func main() {
	client := helpers.CreateKubeConfig()

	namespace := flag.String("ns", "perf", "The namespace to look for the VMs or VDs, default 'perf'")
	getVD := flag.Bool("vd", false, "Get VDs, default false")
	getVM := flag.Bool("vm", false, "Get VMs, default false")
	flag.Parse()

	// err := helpers.SaveToCSV()
	// if err != nil {
	// 	os.Exit(1)
	// }

	ns := *namespace

	if *getVM {
		vm.Get(client, ns)
	} else if *getVD {
		vd.Get(client, ns)
	} else {
		vm.Get(client, ns)
		vd.Get(client, ns)
	}
}
