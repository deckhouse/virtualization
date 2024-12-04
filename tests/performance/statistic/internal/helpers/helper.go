package helpers

import (
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

func DurationToString(d *metav1.Duration) string {
	if d == nil {
		return ""
	}
	return d.Duration.String()
}

func SaveToFile(content string, resType string, ns string) {
	filepath := fmt.Sprintf("/%s-%s-%s.csv", resType, ns, time.Now().Format("2006-01-02_15-04-05"))
	execpath, err := os.Getwd()
	if err != nil {
		os.Exit(1)
	}
	file, err := os.Create(execpath + filepath)
	if err != nil {
		fmt.Printf("Error creating file: %v\n", err)
		return
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		fmt.Printf("Error writing to file: %v\n", err)
		return
	}
}
