package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	log "log/slog"
	"mime"
	"net/http"

	"github.com/tidwall/gjson"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/streaming"
	apiutilnet "k8s.io/apimachinery/pkg/util/net"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"

	logutil "kube-api-proxy/pkg/log"
	"kube-api-proxy/pkg/rewriter"
)

// StreamHandler reads a stream from the target, transforms events
// and sends them to the client.
type StreamHandler struct {
	r         io.ReadCloser
	w         io.Writer
	rewriter  *rewriter.RuleBasedRewriter
	targetReq *rewriter.TargetRequest
	decoder   streaming.Decoder
	done      chan struct{}
	log       *log.Logger
}

// NewStreamHandler starts a go routine to pass rewritten Watch Events
// from server to client.
// Sources:
// k8s.io/apimachinery@v0.26.1/pkg/watch/streamwatcher.go:100 receive method
// k8s.io/kubernetes@v1.13.0/staging/src/k8s.io/client-go/rest/request.go:537 wrapperFn, create framer.
// k8s.io/kubernetes@v1.13.0/staging/src/k8s.io/client-go/rest/request.go:598 instantiate watch NewDecoder
func NewStreamHandler(r io.ReadCloser, w io.Writer, contentType string, rewriter *rewriter.RuleBasedRewriter, targetReq *rewriter.TargetRequest, logger *log.Logger) (*StreamHandler, error) {
	reader := logutil.NewReaderLogger(r)
	wsr := &StreamHandler{
		r:         reader,
		w:         w,
		rewriter:  rewriter,
		targetReq: targetReq,
		done:      make(chan struct{}),
		log:       logger,
	}
	decoder, err := wsr.createWatchDecoder(contentType)
	if err != nil {
		return nil, err
	}
	wsr.decoder = decoder

	// Start stream proxying.
	go wsr.proxy()
	return wsr, nil
}

// proxy reads result from the decoder in a loop, rewrites and writes to a client.
// Sources
// k8s.io/apimachinery@v0.26.1/pkg/watch/streamwatcher.go:100 receive method
func (s *StreamHandler) proxy() {
	defer utilruntime.HandleCrash()
	defer s.Stop()
	for {
		// Read event from the server.
		var got metav1.WatchEvent
		s.log.Info("Start decode from stream")
		res, _, err := s.decoder.Decode(nil, &got)
		s.log.Info("Got decoded WatchEvent from stream")
		if err != nil {
			switch err {
			case io.EOF:
				// watch closed normally
				s.log.Info("Catch EOF from target, stop proxying the stream")
			case io.ErrUnexpectedEOF:
				s.log.Error("Unexpected EOF during watch stream event decoding", logutil.SlogErr(err))
			default:
				if apiutilnet.IsProbableEOF(err) || apiutilnet.IsTimeout(err) {
					s.log.Error("Unable to decode an event from the watch stream", logutil.SlogErr(err))
				} else {
					s.log.Error("Unable to decode an event from the watch stream", logutil.SlogErr(err))
					//select {
					//case <-sw.done:
					//case sw.result <- Event{
					//	Type:   Error,
					//	Object: sw.reporter.AsObject(fmt.Errorf("unable to decode an event from the watch stream: %v", err)),
					//}:
					//}
				}
			}
			//s.log.Info("captured bytes from the stream", logutil.HeadStringEx(s.r, 65536))
			return
		}

		if res != &got {
			s.log.Error("unable to decode to metav1.Event")
			continue
		}

		switch got.Type {
		case string(watch.Added), string(watch.Modified), string(watch.Deleted), string(watch.Error), string(watch.Bookmark):
		default:
			s.log.Error(fmt.Sprintf("got invalid watch event type: %v", got.Type))
			continue
		}

		{
			group := gjson.GetBytes(got.Object.Raw, "apiVersion").String()
			kind := gjson.GetBytes(got.Object.Raw, "kind").String()
			name := gjson.GetBytes(got.Object.Raw, "metadata.name").String()
			ns := gjson.GetBytes(got.Object.Raw, "metadata.namespace").String()
			s.log.Info(fmt.Sprintf("Receive '%s' watch event with %s/%s %s/%s object", got.Type, group, kind, ns, name))
		}

		// TODO add pass-as-is for non rewritable objects.

		// Restore object. Watch responses are always from the Kubernetes API server, no need to rename.
		objBytes, err := s.rewriter.RewriteJSONPayload(s.targetReq, got.Object.Raw, rewriter.Restore)
		//var objBytes []byte
		//switch {
		//case s.targetReq.IsCore():
		//	s.log.Info(fmt.Sprintf("Watch REWRITE CORE event '%s'", got.Type))
		//	objBytes, err = rewriter.RewriteOwnerReferences(s.rewriter.Rules, got.Object.Raw, rewriter.Restore)
		//case s.targetReq.IsCRD():
		//	s.log.Info(fmt.Sprintf("Watch REWRITE CRD event '%s'", got.Type))
		//	objBytes, err = rewriter.RewriteCRDOrList(s.rewriter.Rules, got.Object.Raw, rewriter.Restore, s.targetReq.OrigGroup())
		//case s.targetReq.OrigResourceType() != "":
		//	s.log.Info(fmt.Sprintf("Watch REWRITE event '%s'", got.Type))
		//	objBytes, err = rewriter.RestoreResource(s.rewriter.Rules, got.Object.Raw, s.targetReq.OrigGroup())
		//default:
		//	objBytes = got.Object.Raw
		//}
		if err != nil {
			s.log.Error(fmt.Sprintf("rewrite event '%s'", got.Type), logutil.SlogErr(err))
			continue
		}

		// Write event to the client.
		ev := metav1.WatchEvent{
			Type: got.Type,
			Object: runtime.RawExtension{
				Raw: objBytes,
			},
		}
		evBytes, err := json.Marshal(ev)
		if err != nil {
			s.log.Error("encode restored event to bytes", logutil.SlogErr(err))
			continue
		}
		l := len(evBytes)
		if l > 1300 {
			l = 1300
		}
		s.log.Info(fmt.Sprintf("restored event: %s", string(evBytes)[0:l]))

		_, err = io.Copy(s.w, io.NopCloser(bytes.NewBuffer(evBytes)))
		if err != nil {
			s.log.Error("write event", logutil.SlogErr(err))
		}
		// Flush to send buffered content to the client.
		if wr, ok := s.w.(http.Flusher); ok {
			wr.Flush()
		}

		// Check if application is stopped.
		select {
		case <-s.done:
			return
		default:
		}
	}
}

func (s *StreamHandler) Stop() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
}

func (s *StreamHandler) DoneChan() chan struct{} {
	return s.done
}

// createSerializers
// Source
// k8s.io/client-go@v0.26.1/rest/request.go:765 newStreamWatcher
// k8s.io/apimachinery@v0.26.1/pkg/runtime/negotiate.go:70 StreamDecoder
func (s *StreamHandler) createWatchDecoder(contentType string) (streaming.Decoder, error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		s.log.Info("Unexpected media type from the server: %q: %v", contentType, err)
	}

	negotiatedSerializer := scheme.Codecs.WithoutConversion()
	mediaTypes := negotiatedSerializer.SupportedMediaTypes()
	info, ok := runtime.SerializerInfoForMediaType(mediaTypes, mediaType)
	if !ok {
		if len(contentType) != 0 || len(mediaTypes) == 0 {
			return nil, fmt.Errorf("no matching serializer for media type '%s'", contentType)
		}
		info = mediaTypes[0]
	}
	if info.StreamSerializer == nil {
		return nil, fmt.Errorf("no serializer for content type %s", contentType)
	}

	// A chain of the framer and the serializer will split body stream into JSON objects.
	frameReader := info.StreamSerializer.Framer.NewFrameReader(s.r)
	streamingDecoder := streaming.NewDecoder(frameReader, info.StreamSerializer.Serializer)
	return streamingDecoder, nil
}
