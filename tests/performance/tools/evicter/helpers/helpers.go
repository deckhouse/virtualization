package helpers

import (
	"fmt"
	"os"
	"strings"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"k8s.io/client-go/tools/clientcmd"
)

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
