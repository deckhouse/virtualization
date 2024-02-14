package main

import (
	"flag"
	"fmt"
	virtv2 "github.com/deckhouse/virtualization-controller/api/v1alpha2"
	"github.com/deckhouse/virtualization-controller/pkg/apiserver"
	"go.uber.org/zap/zapcore"
	"os"
	"sigs.k8s.io/apiserver-runtime/pkg/builder"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"strconv"
)

var (
	log = logf.Log.WithName("cmd")
)

const (
	defaultVerbosity = "1"
)

func setupLogger() {
	verbose := defaultVerbosity
	if verboseEnvVarVal := os.Getenv("VERBOSITY"); verboseEnvVarVal != "" {
		verbose = verboseEnvVarVal
	}
	// visit actual flags passed in and if passed check -v and set verbose
	if fv := flag.Lookup("v"); fv != nil {
		verbose = fv.Value.String()
	}
	if verbose == defaultVerbosity {
		log.V(1).Info(fmt.Sprintf("Note: increase the -v level in the controller deployment for more detailed logging, eg. -v=%d or -v=%d\n", 2, 3))
	}
	verbosityLevel, err := strconv.Atoi(verbose)
	debug := false
	if err == nil && verbosityLevel > 1 {
		debug = true
	}

	// The logger instantiated here can be changed to any logger
	// implementing the logr.Logger interface. This logger will
	// be propagated through the whole operator, generating
	// uniform and structured logs.
	logf.SetLogger(zap.New(zap.Level(zapcore.Level(-1*verbosityLevel)), zap.UseDevMode(debug)))
}

func main() {
	flag.Parse()
	setupLogger()
	if _, err := apiserver.GetConf(); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	err := builder.APIServer.
		WithResource(&virtv2.VirtualMachine{}).
		WithLocalDebugExtension().
		ExposeLoopbackClientConfig().
		WithOptionsFns(func(options *builder.ServerOptions) *builder.ServerOptions {
			options.RecommendedOptions.CoreAPI = nil
			options.RecommendedOptions.Admission = nil
			return options
		}).
		Execute()

	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}
}
