package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
)

func main() {
	// kubeclient.DefaultClientConfig() prepares config using kubeconfig.
	// typically, you need to set env variable, KUBECONFIG=<path-to-kubeconfig>/.kubeconfig
	clientConfig := kubeclient.DefaultClientConfig(&pflag.FlagSet{})

	// retrive default namespace.
	namespace, _, err := clientConfig.Namespace()
	if err != nil {
		log.Fatalf("error in namespace : %v\n", err)
	}

	// get the virtualization client
	client, err := kubeclient.GetClientFromClientConfig(clientConfig)
	if err != nil {
		log.Fatalf("Cannot obtain Virtualization client: %v\n", err)
	}

	// In all namespaces - namespace should empty
	// Fetch list of VirtualMachines.
	vmList, err := client.VirtualMachines(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Cannot fetch VirtualMachines in namespace %s: %v", namespace, err)
	}
	// Fetch list of Disks.
	diskList, err := client.VirtualDisks(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Cannot fetch Disks in namespace %s: %v", namespace, err)
	}
	// Fetch list of Images.
	imgList, err := client.VirtualImages(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Cannot fetch Images in namespace %s: %v", namespace, err)
	}
	// Fetch list of ClusterImages.
	cimgList, err := client.ClusterVirtualImages().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Cannot fetch ClusterImages: %v", err)
	}
	// Fetch list of IPAddressClaims.
	ipcList, err := client.VirtualMachineIPAddressClaims(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Cannot fetch IPAddressClaims in namespace %s: %v", namespace, err)
	}
	// Fetch list of VirtualMachineOperations.
	opsList, err := client.VirtualMachineOperations(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Cannot fetch VirtualMachineOperations in namespace %s: %v", namespace, err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 5, ' ', 0)
	fmt.Fprintln(w, "Type\tName\tNamespace\tStatus")

	for _, obj := range vmList.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%v\n", obj.Kind, obj.Name, obj.Namespace, obj.Status.Phase)
	}
	for _, obj := range diskList.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%v\n", obj.Kind, obj.Name, obj.Namespace, obj.Status.Phase)
	}
	for _, obj := range imgList.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%v\n", obj.Kind, obj.Name, obj.Namespace, obj.Status.Phase)
	}
	for _, obj := range cimgList.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%v\n", obj.Kind, obj.Name, "-", obj.Status.Phase)
	}
	for _, obj := range ipcList.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%v\n", obj.Kind, obj.Name, obj.Namespace, obj.Status.Phase)
	}
	for _, obj := range opsList.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%v\n", obj.Kind, obj.Name, obj.Namespace, obj.Status.Phase)
	}
	w.Flush()
}
