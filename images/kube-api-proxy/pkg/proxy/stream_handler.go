package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	log "log/slog"
	"mime"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
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

type StreamHandler struct {
	r         io.ReadCloser
	w         io.Writer
	origGroup string
	rewriter  *rewriter.RuleBasedRewriter
	decoder   streaming.Decoder
	done      chan struct{}
}

// NewStreamHandler starts a go routine to pass rewritten Watch Events
// from server to client.
// Sources:
// k8s.io/apimachinery@v0.26.1/pkg/watch/streamwatcher.go:100 receive method
// k8s.io/kubernetes@v1.13.0/staging/src/k8s.io/client-go/rest/request.go:537 wrapperFn, create framer.
// k8s.io/kubernetes@v1.13.0/staging/src/k8s.io/client-go/rest/request.go:598 instantiate watch NewDecoder
func NewStreamHandler(r io.ReadCloser, w io.Writer, contentType string, origGroup string, rewriter *rewriter.RuleBasedRewriter) (*StreamHandler, error) {
	wsr := &StreamHandler{
		r:         r,
		w:         w,
		origGroup: origGroup,
		rewriter:  rewriter,
		done:      make(chan struct{}),
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
func (wsr *StreamHandler) proxy() {
	defer utilruntime.HandleCrash()
	defer wsr.Stop()
	for {
		// Read event from the server.
		var got metav1.WatchEvent
		res, _, err := wsr.decoder.Decode(nil, &got)
		if err != nil {
			switch err {
			case io.EOF:
				// watch closed normally
				log.Info("Catch EOF, stop proxying")
			case io.ErrUnexpectedEOF:
				log.Error("Unexpected EOF during watch stream event decoding", logutil.SlogErr(err))
			default:
				if apiutilnet.IsProbableEOF(err) || apiutilnet.IsTimeout(err) {
					log.Error("Unable to decode an event from the watch stream", logutil.SlogErr(err))
				} else {
					log.Error("Unable to decode an event from the watch stream", logutil.SlogErr(err))
					//select {
					//case <-sw.done:
					//case sw.result <- Event{
					//	Type:   Error,
					//	Object: sw.reporter.AsObject(fmt.Errorf("unable to decode an event from the watch stream: %v", err)),
					//}:
					//}
				}
			}
			return
		}

		if res != &got {
			log.Error("unable to decode to metav1.Event")
			continue
		}

		switch got.Type {
		case string(watch.Added), string(watch.Modified), string(watch.Deleted), string(watch.Error), string(watch.Bookmark):
		default:
			log.Error(fmt.Sprintf("got invalid watch event type: %v", got.Type))
			continue
		}

		{
			group := gjson.GetBytes(got.Object.Raw, "apiVersion").String()
			kind := gjson.GetBytes(got.Object.Raw, "kind").String()
			name := gjson.GetBytes(got.Object.Raw, "metadata.name").String()
			ns := gjson.GetBytes(got.Object.Raw, "metadata.namespace").String()
			log.Info(fmt.Sprintf("Proxy '%s' event with %s/%s %s/%s object", got.Type, group, kind, ns, name))
		}

		// Rewrite object in the event.
		rwrBytes, err := wsr.rewriter.RestoreResource(got.Object.Raw, wsr.origGroup)
		if err != nil {
			log.Error(fmt.Sprintf("rewrite event '%s'", got.Type), logutil.SlogErr(err))
			continue
		}

		// Write event to the client.
		ev := metav1.WatchEvent{
			Type: got.Type,
			Object: runtime.RawExtension{
				Raw: rwrBytes,
			},
		}
		evBytes, err := json.Marshal(ev)
		if err != nil {
			log.Error("encode restored event to bytes", logutil.SlogErr(err))
			continue
		}
		l := len(evBytes)
		if l > 300 {
			l = 300
		}
		log.Info(fmt.Sprintf("restored event: %s", string(evBytes)[0:l]))
		wsr.w.Write(evBytes)
		select {
		case <-wsr.done:
			return
		default:
		}
	}
}

func (wsr *StreamHandler) Stop() {
	select {
	case <-wsr.done:
	default:
		close(wsr.done)
	}
}

func (wsr *StreamHandler) DoneChan() chan struct{} {
	return wsr.done
}

// restoreWatchEvent restores renamed kind and apiVersion to original values.
func restoreWatchEvent(ev *metav1.WatchEvent) ([]byte, error) {
	kind := gjson.GetBytes(ev.Object.Raw, "kind").String()

	var rwrBytes []byte
	var err error
	switch kind {
	default:
		return nil, nil
	case "VirtualMachine":
		rwrBytes, err = sjson.SetBytes(ev.Object.Raw, "kind", "VirtualMachine")
		if err != nil {
			return nil, err
		}
		rwrBytes, err = sjson.SetBytes(rwrBytes, "apiVersion", "kubevirt.io/v1")
		if err != nil {
			return nil, err
		}
	}
	// TODO: rewrite obj by kind.
	// No rewrite for now, return as-is.
	return rwrBytes, nil
}

// createSerializers
// Source
// k8s.io/client-go@v0.26.1/rest/request.go:765 newStreamWatcher
// k8s.io/apimachinery@v0.26.1/pkg/runtime/negotiate.go:70 StreamDecoder
func (wsr *StreamHandler) createWatchDecoder(contentType string) (streaming.Decoder, error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		log.Info("Unexpected media type from the server: %q: %v", contentType, err)
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
	frameReader := info.StreamSerializer.Framer.NewFrameReader(wsr.r)
	streamingDecoder := streaming.NewDecoder(frameReader, info.StreamSerializer.Serializer)
	return streamingDecoder, nil
}
