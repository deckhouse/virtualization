package exporter

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"k8s.io/klog/v2"
)

type exportServer struct {
	tlsKeyPath  string
	tlsCertPath string
	username    string
	password    string
	image       string
	bindAddress string
	bindPort    int
	insecure    bool

	done chan struct{}
}

func NewExportServer(image string, bindAddress string, bindPort int, options ...Option) Exporter {
	s := &exportServer{
		image:       image,
		bindAddress: bindAddress,
		bindPort:    bindPort,

		done: make(chan struct{}, 1),
	}
	for _, option := range options {
		option(s)
	}

	return s
}

type Option func(*exportServer)

func WithTLS(tlsKeyPath string, tlsCertPath string) Option {
	return func(s *exportServer) {
		s.tlsKeyPath = tlsKeyPath
		s.tlsCertPath = tlsCertPath
	}
}

func WithAuth(username string, password string) Option {
	return func(s *exportServer) {
		s.username = username
		s.password = password
	}
}

func WithInsecure(insecure bool) Option {
	return func(s *exportServer) {
		s.insecure = insecure
	}
}

func (s *exportServer) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.bindAddress, s.bindPort))
	if err != nil {
		return fmt.Errorf("error creating export listerner: %w", err)
	}

	srv := &http.Server{
		Handler: s.getHandler(),
	}

	go func() {
		select {
		case <-ctx.Done():
		case <-s.done:
		}
		klog.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			klog.Error(err, "error shutting down server")
		}
	}()

	if s.tlsKeyPath != "" && s.tlsCertPath != "" {
		err = srv.ServeTLS(listener, s.tlsCertPath, s.tlsKeyPath)
	} else {
		err = srv.Serve(listener)
	}

	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *exportServer) getHandler() http.Handler {
	started := false
	mux := http.NewServeMux()
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		ref, err := name.ParseReference(s.image, s.dvcrNameOptions()...)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid image reference: %v", err), http.StatusBadRequest)
			return
		}

		img, err := remote.Image(ref, s.dvcrRemoteOptions(r.Context())...)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get image: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.tar", strings.ReplaceAll(ref.Name(), "/", "_")))
		w.Header().Set("Content-Type", "application/octet-stream")

		started = true

		err = crane.Export(img, w)
		if err != nil {
			started = false
			http.Error(w, fmt.Sprintf("Failed to export image: %v", err), http.StatusInternalServerError)
			return
		}
		s.done <- struct{}{}
	})
	mux.HandleFunc("/started", func(w http.ResponseWriter, r *http.Request) {
		msg := "no"
		if started {
			msg = "yes"
		}
		_, err := w.Write([]byte(msg))
		if err != nil {
			klog.Error(err, "failed to write response")
			w.WriteHeader(http.StatusInternalServerError)
		}
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

func (s *exportServer) dvcrRemoteOptions(ctx context.Context) []remote.Option {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: s.insecure,
	}

	remoteOpts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithTransport(transport),
	}
	if s.username != "" && s.password != "" {
		remoteOpts = append(remoteOpts, remote.WithAuth(&authn.Basic{Username: s.username, Password: s.password}))
	}

	return remoteOpts
}

func (s *exportServer) dvcrNameOptions() []name.Option {
	nameOptions := []name.Option{
		name.StrictValidation,
	}

	if s.insecure {
		nameOptions = append(nameOptions, name.Insecure)
	}

	return nameOptions
}
