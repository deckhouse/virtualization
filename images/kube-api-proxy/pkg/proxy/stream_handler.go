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

		var rwrEvent *metav1.WatchEvent
		if res != &got {
			s.log.Warn(fmt.Sprintf("unable to decode to metav1.Event: res=%#v, got=%#v", res, got))
			// There is nothing to send to the client: no event decoded.
		} else {
			rwrEvent, err = s.transformWatchEvent(&got)
			if err != nil {
				s.log.Error(fmt.Sprintf("Watch event '%s': transform error", got.Type), logutil.SlogErr(err))
				logutil.DebugBodyHead(s.log, fmt.Sprintf("Watch event '%s'", got.Type), s.targetReq.OrigResourceType(), got.Object.Raw)
			}
			if rwrEvent == nil {
				// No rewrite, pass original event as-is.
				rwrEvent = &got
			} else {
				// Log changes after rewrite.
				logutil.DebugBodyChanges(s.log, "Watch event", s.targetReq.OrigResourceType(), got.Object.Raw, rwrEvent.Object.Raw)
			}
			// Pass event to the client.
			logutil.DebugBodyHead(s.log, fmt.Sprintf("WatchEvent type '%s' send back to client %d bytes", rwrEvent.Type, len(rwrEvent.Object.Raw)), s.targetReq.OrigResourceType(), rwrEvent.Object.Raw)
			s.writeEvent(rwrEvent)
		}

		// Check if application is stopped before waiting for the next event.
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

func (s *StreamHandler) transformWatchEvent(ev *metav1.WatchEvent) (*metav1.WatchEvent, error) {
	switch ev.Type {
	case string(watch.Added), string(watch.Modified), string(watch.Deleted), string(watch.Error), string(watch.Bookmark):
	default:
		return nil, fmt.Errorf("got unknown type: %v", ev.Type)
	}

	group := gjson.GetBytes(ev.Object.Raw, "apiVersion").String()
	kind := gjson.GetBytes(ev.Object.Raw, "kind").String()
	name := gjson.GetBytes(ev.Object.Raw, "metadata.name").String()
	ns := gjson.GetBytes(ev.Object.Raw, "metadata.namespace").String()

	// TODO add pass-as-is for non rewritable objects.
	if group == "" && kind == "" {
		// Object in event is undetectable, pass this event as-is.
		return nil, fmt.Errorf("object has no apiVersion and kind")
	}
	s.log.Debug(fmt.Sprintf("Receive '%s' watch event with %s/%s %s/%s object", ev.Type, group, kind, ns, name))

	// Restore object in the event. Watch responses are always from the Kubernetes API server, so rename is not needed.
	rwrObjBytes, err := s.rewriter.RewriteJSONPayload(s.targetReq, ev.Object.Raw, rewriter.Restore)
	if err != nil {
		return nil, fmt.Errorf("error rewriting object: %w", err)
	}

	// Prepare rewritten event bytes.
	return &metav1.WatchEvent{
		Type: ev.Type,
		Object: runtime.RawExtension{
			Raw: rwrObjBytes,
		},
	}, nil
}

func (s *StreamHandler) writeEvent(ev *metav1.WatchEvent) {
	rwrEventBytes, err := json.Marshal(ev)
	if err != nil {
		s.log.Error("encode restored event to bytes", logutil.SlogErr(err))
		return
	}

	// Send rewritten event to the client.
	_, err = io.Copy(s.w, io.NopCloser(bytes.NewBuffer(rwrEventBytes)))
	if err != nil {
		s.log.Error("Watch event: error writing event to the client", logutil.SlogErr(err))
	}
	// Flush to send buffered content to the client.
	if wr, ok := s.w.(http.Flusher); ok {
		wr.Flush()
	}
}
