package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gorilla/websocket"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	//-----------------------------------------------------------------
	// 1. kube‑config from ~/.kube/config
	cfgFile := fmt.Sprintf("%s/.kube/config", os.Getenv("HOME"))
	kcfg, err := clientcmd.BuildConfigFromFlags("", cfgFile)
	if err != nil {
		log.Fatalf("kubeconfig: %v", err)
	}

	//-----------------------------------------------------------------
	// 2. Собираем wss‑URL к subresource usbredir
	ns, name := "test-project", "linux-vm"
	host := strings.TrimPrefix(kcfg.Host, "https://")
	wsURL := url.URL{
		Scheme: "wss",
		Host:   host,
		// Path: fmt.Sprintf("/apis/subresources.kubevirt.io/v1/namespaces/%s/"+
		// 	"virtualmachineinstances/%s/usbredir", ns, name),
		Path: fmt.Sprintf("/apis/subresources.virtualization.deckhouse.io/v1alpha2/namespaces/%s/"+
			"virtualmachines/%s/usbredir", ns, name),
		// Path: "/apis/subresources.virtualization.deckhouse.io/v1alpha2/namespaces/test-project/virtualmachines/linux-vm/usbredir",
		// kvvmiPathTmpl = "/apis/subresources.kubevirt.io/v1/namespaces/%s/virtualmachineinstances/%s/%s"
	}
	fmt.Println(wsURL)

	//-----------------------------------------------------------------
	// 3. Создаём TLS‑конфиг из kube‑config  (функция rest.TLSConfigFor)
	tlsCfg, err := rest.TLSConfigFor(kcfg) // :contentReference[oaicite:0]{index=0}
	if err != nil {
		log.Fatalf("TLS config: %v", err)
	}

	dialer := websocket.Dialer{
		TLSClientConfig: tlsCfg,
		Subprotocols:    []string{""},
	}

	//-----------------------------------------------------------------
	// 4. TCP‑порт 4000 — сюда подключается usbredirect‑клиент
	l, err := net.Listen("tcp", "127.0.0.1:4000")
	if err != nil {
		log.Fatalf("listen 4000: %v", err)
	}
	log.Printf("usbredir proxy ready on localhost:4000")

	for {
		tcpConn, err := l.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go handle(tcpConn, dialer, wsURL, kcfg.BearerToken)
	}
}

// handle: на каждое TCP‑подключение открываем свой WebSocket
func handle(tcp net.Conn, dialer websocket.Dialer, wsURL url.URL, token string) {
	defer tcp.Close()

	hdr := http.Header{}
	if token != "" {
		hdr.Set("Authorization", "Bearer "+token)
	}

	ws, _, err := dialer.Dial(wsURL.String(), hdr)
	if err != nil {
		log.Printf("dial websocket: %v", err)
		return
	}
	defer ws.Close()

	// WebSocket <‑‑> TCP копируем параллельно
	go func() {
		// TCP → WS
		for {
			buf := make([]byte, 32*1024)
			n, err := tcp.Read(buf)
			if n > 0 {
				if werr := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(
			websocket.CloseNormalClosure, ""))
	}()

	// WS → TCP
	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			break
		}
		if _, err := tcp.Write(data); err != nil {
			break
		}
	}
	log.Printf("client disconnected")
}
