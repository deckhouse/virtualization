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

package server

type Option interface {
	Apply(*options)
}

type options struct {
	TLS *TLSPair
}

type TLSPair struct {
	CaFile   string
	CertFile string
	KeyFile  string
}

func WithTLS(caFile, certFile, keyFile string) Option {
	return tlsOption{
		CertFile: certFile,
		KeyFile:  keyFile,
		CaFile:   caFile,
	}
}

type tlsOption struct {
	CertFile string
	KeyFile  string
	CaFile   string
}

func (t tlsOption) Apply(o *options) {
	o.TLS = &TLSPair{
		CertFile: t.CertFile,
		KeyFile:  t.KeyFile,
		CaFile:   t.CaFile,
	}
}
