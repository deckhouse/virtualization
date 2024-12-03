package main

import (
	"context"
	"flag"

	// "fmt"
	"math/rand"
	// "os"
	"log"

	"evicter/helpers"

	// core "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources/v1alpha2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	// "k8s.io/client-go/kubernetes"
	// "k8s.io/client-go/tools/clientcmd"
	// "k8s.io/client-go/util/homedir"
	// "path/filepath"
	"time"
)

var (
	namespace string
	target    int
)

func main() {
	// create client
	client := helpers.CreateKubeConfig()
	// parse flags
	flag.StringVar(&namespace, "ns", "perf", "The namespace to look for the VMs or VDs, default 'perf'")
	flag.IntVar(&target, "target", 10, "Target percentage for VM evict")
	flag.Parse()

	vmTracked := make(map[string]time.Time)
	// main loop
	for {
		// get all VMs
		vmListTotal, err := client.VirtualMachines(namespace).List(context.TODO(), metav1.ListOptions{})
		vmTotal := len(vmListTotal.Items)
		log.Printf("Total VMs: %d\n", vmTotal)
		if err != nil {
			log.Fatal(err)
		}

		// calculate target number of VMs to evict
		vmNumTargetInMigration := target * vmTotal / 100

		// get all migrating VMs
		vmMigrating := make(map[string]bool)
		for _, vm := range vmListTotal.Items {
			if vm.Status.Phase == "Migrating" {
				vmMigrating[vm.Name] = true
			}
		}

		// delete migrating VMs from tracked VMs
		for name := range vmMigrating {
			delete(vmTracked, name)
			log.Printf("Tracked VM in Migrating phase: %s\n", name)
		}
		// delete VMs who are in Running phase for more than 30s, we probably missed it
		for name, t := range vmTracked {
			if time.Since(t) > 30*time.Second {
				delete(vmTracked, name)
				log.Printf("Tracked VM in Running phase for more than 30s: %s\n", name)
			}
		}
		vmNumTargetInMigration -= len(vmTracked)
		log.Printf("Target number of VMs to evict: %d\n", vmNumTargetInMigration)

		// populate vmTracked with random Running VMs
		for i := 0; i < vmNumTargetInMigration; i++ {
			// try until find Running VM
			for {
				vmIndex := rand.Intn(vmTotal)
				_, exists := vmMigrating[vmListTotal.Items[vmIndex].Name]
				if !exists {
					vmTracked[vmListTotal.Items[vmIndex].Name] = time.Now()
					client.VirtualMachines(namespace).Migrate(context.TODO(), vmListTotal.Items[vmIndex].Name, v1alpha2.VirtualMachineMigrate{})
					log.Printf("VM to evict: %s\n", vmListTotal.Items[vmIndex].Name)
					break
				}
			}
		}
		log.Printf("Total number of tracked VMs: %d\n", len(vmTracked))
		log.Println("Tracked VMs:")
		for k := range vmTracked {
			log.Printf("VM: %s\n", k)
		}
	}
}
