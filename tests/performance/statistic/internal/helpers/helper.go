package helpers

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

func ToSeconds(duration *metav1.Duration) float64 {
	if duration == nil {
		return 0
	}
	return duration.Seconds()
}

func CreateKubeConfig() kubeclient.Client {
	// Get the KUBECONFIG environment variable
	kubeconfigEnv := os.Getenv("KUBECONFIG")
	if kubeconfigEnv == "" {

		fmt.Println("Try to use default path $HOME/.kube/config")
		userHomeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Println("Failed to get user home directory:", err)
			os.Exit(1)
		}
		kubeconfigEnv = userHomeDir + "/.kube/config"

		_ = os.Setenv("KUBECONFIG", kubeconfigEnv)
		kubeconfigEnv = os.Getenv("KUBECONFIG")

		if kubeconfigEnv == "" {
			fmt.Println("KUBECONFIG environment variable is not set. Exiting.")
			os.Exit(1)
		}
	}

	// Split the KUBECONFIG environment variable (handles merged kubeconfig paths)
	kubeconfigPaths := strings.Split(kubeconfigEnv, string(os.PathListSeparator))
	if len(kubeconfigPaths) == 0 {
		fmt.Println("No valid kubeconfig paths found in KUBECONFIG. Exiting.")
		os.Exit(1)
	}

	fmt.Printf("Using KUBECONFIG paths: %v\n", kubeconfigPaths)

	// Load the kubeconfig from the merged paths
	loadingRules := &clientcmd.ClientConfigLoadingRules{
		Precedence: kubeconfigPaths,
	}
	clientConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		fmt.Printf("Failed to load kubeconfig: %v\n", err)
		os.Exit(1)
	}

	// Create a Kubernetes client
	client, err := kubeclient.GetClientFromRESTConfig(clientConfig)
	if err != nil {
		fmt.Printf("Failed to create Kubernetes client: %v\n", err)
		os.Exit(1)
	}
	return client
}

// vd.Name vd.WaitingForDependencies vd.DVCRProvisioning vd.TotalProvisioning
// vm.Name vm.WaitingForDependencies vm.VirtualMachineStarting vm.GuestOSAgentStarting
// func SaveToCSV(data []string, path string) error {
func SaveToCSV(header []string, data struct{}) error {
	logFiile := "/log-" + time.Now().Format("2006-01-02_15-04-05") + ".csv"
	// execpath, err := os.Executable()
	execpath, err := os.Getwd()
	if err != nil {
		return err
	}
	// exPath := filepath.Dir(execpath)
	// fmt.Println(exPath)
	// fmt.Println(execpath + logFiile)

	file, err := os.Create(execpath + logFiile)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header row
	if err := writer.Write(header); err != nil {
		fmt.Printf("Error writing header to CSV file: %v\n", err)
		os.Exit(1)
	}

	// Write pod details
	// for _, res := range data. {
	// 	row := []string{
	// 		res,
	// 	}

	// 	if err := writer.Write(row); err != nil {
	// 		fmt.Printf("Error writing row to CSV file: %v\n", err)
	// 		os.Exit(1)
	// 	}
	// }

	return nil
}
