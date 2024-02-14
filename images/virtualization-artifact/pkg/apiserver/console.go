package apiserver

import (
	"k8s.io/client-go/kubernetes"
	"net/http"
)

type ConsoleHandler struct {
	kubernetes.Clientset
}

func NewConsoleHandler(client kubernetes.Clientset) *ConsoleHandler {
	return &ConsoleHandler{
		Clientset: client,
	}
}

func (h *ConsoleHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

}
