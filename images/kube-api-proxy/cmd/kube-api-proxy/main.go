/*
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	log "log/slog"
	"os"

	"kube-api-proxy/pkg/kubevirt"
	logutil "kube-api-proxy/pkg/log"
	"kube-api-proxy/pkg/proxy"
	"kube-api-proxy/pkg/rewriter"
	"kube-api-proxy/pkg/server"
	"kube-api-proxy/pkg/target"
)

// This proxy is a proof-of-concept of proxying Kubernetes API requests
// with rewrites.
//
// It assumes presence of KUBERNETES_* environment variables and files
// in /var/run/secrets/kubernetes.io/serviceaccount (token and ca.crt).
//
// A client behind the proxy should connect to 127.0.0.1:$PROXY_PORT
// using plain http. Example of kubeconfig file:
// apiVersion: v1
// kind: Config
// clusters:
// - cluster:
//   server: http://127.0.0.1:23915
//   name: proxy.api.server
// contexts:
// - context:
//   cluster: proxy.api.server
//   name: proxy.api.server
// current-context: proxy.api.server

const (
	loopbackAddr              = "127.0.0.1"
	anyAddr                   = "0.0.0.0"
	defaultAPIClientProxyPort = "23915"
	defaultWebhookProxyPort   = "24192"
)

const (
	logLevelEnv  = "LOG_LEVEL"
	logFormatEnv = "LOG_FORMAT"
	logOutputEnv = "LOG_OUTPUT"
)

func main() {
	// Set options for the default logger: level, format and output.
	logutil.SetupDefaultLoggerFromEnv(logutil.Options{
		Level:  os.Getenv(logLevelEnv),
		Format: os.Getenv(logFormatEnv),
		Output: os.Getenv(logOutputEnv),
	})

	// Load rules from file or use default kubevirt rules.
	rewriteRules := kubevirt.KubevirtRewriteRules
	if os.Getenv("RULES_PATH") != "" {
		rulesFromFile, err := rewriter.LoadRules(os.Getenv("RULES_PATH"))
		if err != nil {
			log.Error("Load rules from %s: %v", os.Getenv("RULES_PATH"), err)
			os.Exit(1)
		}
		rewriteRules = rulesFromFile
	}
	rewriteRules.Init()

	proxies := make([]*server.HTTPServer, 0)

	// Register direct proxy from local Kubernetes API client to Kubernetes API server.
	if os.Getenv("CLIENT_PROXY") == "no" {
		log.Info("Will not start client proxy: CLIENT_PROXY=no")
	} else {
		config, err := target.NewKubernetesTarget()
		if err != nil {
			log.Error("Load Kubernetes REST", logutil.SlogErr(err))
			os.Exit(1)
		}
		lAddr := server.ConstructListenAddr(
			os.Getenv("CLIENT_PROXY_ADDRESS"), os.Getenv("CLIENT_PROXY_PORT"),
			loopbackAddr, defaultAPIClientProxyPort)
		rwr := &rewriter.RuleBasedRewriter{
			Rules: rewriteRules,
		}
		proxyHandler := &proxy.Handler{
			Name:         "kube-api",
			TargetClient: config.Client,
			TargetURL:    config.APIServerURL,
			ProxyMode:    proxy.ToRenamed,
			Rewriter:     rwr,
		}
		proxySrv := &server.HTTPServer{
			InstanceDesc: "API Client proxy",
			ListenAddr:   lAddr,
			RootHandler:  proxyHandler,
		}
		proxies = append(proxies, proxySrv)
	}

	// Register reverse proxy from Kubernetes API server to local webhook server.
	if os.Getenv("WEBHOOK_ADDRESS") == "" {
		log.Info("Will not start webhook proxy for empty WEBHOOK_ADDRESS")
	} else {
		config, err := target.NewWebhookTarget()
		if err != nil {
			log.Error("Configure webhook client", logutil.SlogErr(err))
			os.Exit(1)
		}
		lAddr := server.ConstructListenAddr(
			os.Getenv("WEBHOOK_PROXY_ADDRESS"), os.Getenv("WEBHOOK_PROXY_PORT"),
			anyAddr, defaultWebhookProxyPort)
		rwr := &rewriter.RuleBasedRewriter{
			Rules: rewriteRules,
		}
		proxyHandler := &proxy.Handler{
			Name:         "webhook",
			TargetClient: config.Client,
			TargetURL:    config.URL,
			ProxyMode:    proxy.ToOriginal,
			Rewriter:     rwr,
		}
		proxySrv := &server.HTTPServer{
			InstanceDesc: "Webhook proxy",
			ListenAddr:   lAddr,
			RootHandler:  proxyHandler,
			CertManager:  config.CertManager,
		}
		proxies = append(proxies, proxySrv)
	}

	if len(proxies) == 0 {
		log.Info("No proxies to start, exit")
		return
	}

	// Start proxies and block the main process until at least one proxy stops.
	proxyGroup := server.NewRunnableGroup()
	for i := range proxies {
		proxyGroup.Add(proxies[i])
	}
	// Block while proxies are running.
	proxyGroup.Start()

	// Log errors for each instance and exit.
	exitCode := 0
	for _, srv := range proxies {
		if srv.Err != nil {
			log.Error(srv.InstanceDesc, logutil.SlogErr(srv.Err))
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}
