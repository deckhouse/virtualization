/*
Copyright 2025 Flant JSC

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

package exporter

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"
	"k8s.io/klog/v2"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/auth"
	dtls "github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/tls"
)

type exportServer struct {
	image         string
	bindAddress   string
	bindPort      int
	authenticator auth.Authenticator
	authorizer    auth.Authorizer

	tlsKeyPath   string
	tlsCertPath  string
	username     string
	password     string
	destInsecure bool
	destCertPath string

	done chan struct{}

	destTlSConfig *tls.Config
}

func NewExportServer(image string, bindAddress string, bindPort int, authenticator auth.Authenticator, authorizer auth.Authorizer, options ...Option) Exporter {
	s := &exportServer{
		image:         image,
		bindAddress:   bindAddress,
		bindPort:      bindPort,
		authenticator: authenticator,
		authorizer:    authorizer,

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

func WithDestInsecure(insecure bool) Option {
	return func(s *exportServer) {
		s.destInsecure = insecure
	}
}

func WithDestCert(certPath string) Option {
	return func(s *exportServer) {
		s.destCertPath = certPath
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
	if s.destCertPath != "" {
		certPool, err := dtls.NewCertPool(s.destCertPath)
		if err != nil {
			return err
		}
		s.destTlSConfig = dtls.NewBuilder().WithRootCAs(certPool).Build()
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

	klog.Info("starting server")
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
	var count atomic.Int64
	mux := http.NewServeMux()

	mux.Handle("/download", s.withMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.New().String()
		log := klog.LoggerWithValues(klog.Background(), "requestID", requestID, "remoteAddr", r.RemoteAddr, "image", s.image)
		log.Info("Starting download request")

		ref, err := name.ParseReference(s.image, s.dvcrNameOptions()...)
		if err != nil {
			log.Error(err, "Invalid image reference")
			http.Error(w, fmt.Sprintf("Invalid image reference: %v", err), http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		img, err := remote.Image(ref, s.dvcrRemoteOptions(ctx)...)
		if err != nil {
			log.Error(err, "Failed to get image")
			http.Error(w, fmt.Sprintf("Failed to get image: %v", err), http.StatusInternalServerError)
			return
		}

		fileName := strings.ReplaceAll(ref.Name(), "/", "_") + ".tar"
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
		w.Header().Set("Content-Type", "application/x-tar")

		count.Add(1)
		defer func() {
			count.Add(-1)
			log.Info("Download finished")
		}()

		log.Info("Exporting image to tar")

		pw := &progressWriter{
			ResponseWriter: w,
			total:          0,
			lastLogged:     time.Now(),
			log:            &log,
		}

		err = crane.Export(img, pw)
		if err != nil {
			log.Error(err, "Failed to export image")
			http.Error(w, fmt.Sprintf("Failed to export image: %v", err), http.StatusInternalServerError)
			return
		}

		log.Info("Successfully exported image")
	})))

	mux.HandleFunc("/inprogress-count", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(strconv.Itoa(int(count.Load()))))
		if err != nil {
			klog.Error(err, "failed to write response")
		}
	})

	return mux
}

func (s *exportServer) withMiddleware(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authenticateRequest, err := s.authenticator.AuthenticateRequest(r)
		if err != nil {
			klog.Error(err, "Failed to authenticate request")
			http.Error(w, fmt.Sprintf("Failed to authenticate request: %v", err), http.StatusUnauthorized)
			return
		}

		if !authenticateRequest.Authenticated {
			klog.Error(err, "Request is not authenticated")
			http.Error(w, fmt.Sprintf("Request is not authenticated"), http.StatusUnauthorized)
			return
		}

		authorizeResult, err := s.authorizer.Authorize(r.Context(), authenticateRequest.UserName, authenticateRequest.Groups)
		if err != nil {
			klog.Error(err, "Failed to authorize request")
			http.Error(w, fmt.Sprintf("Failed to authorize request: %v", err), http.StatusForbidden)
			return
		}

		if !authorizeResult.Allowed {
			klog.Error(err, "Request is not authorized")
			http.Error(w, fmt.Sprintf("Request is not authorized"), http.StatusForbidden)
			return
		}

		handler.ServeHTTP(w, r)
	})
}

func (s *exportServer) dvcrRemoteOptions(ctx context.Context) []remote.Option {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = s.destTlSConfig
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: s.destInsecure,
		}
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

	if s.destInsecure {
		nameOptions = append(nameOptions, name.Insecure)
	}

	return nameOptions
}

type progressWriter struct {
	http.ResponseWriter
	total      int64
	lastLogged time.Time
	log        *klog.Logger
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.ResponseWriter.Write(p)
	pw.total += int64(n)

	if time.Since(pw.lastLogged) > time.Second {
		pw.log.Info(fmt.Sprintf("Transferred %d bytes...", pw.total))
		pw.lastLogged = time.Now()
	}

	return n, err
}
