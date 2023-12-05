package registry

type ImageInformer struct {
	virtualSize uint64
	format      string

	wait chan struct{}
}

func NewImageInformer() *ImageInformer {
	return &ImageInformer{
		wait: make(chan struct{}),
	}
}

func (r *ImageInformer) Set(virtualSize uint64, format string) {
	r.virtualSize = virtualSize
	r.format = format

	close(r.wait)
}

func (r *ImageInformer) Wait() <-chan struct{} {
	return r.wait
}

func (r *ImageInformer) GetVirtualSize() uint64 {
	return r.virtualSize
}

func (r *ImageInformer) GetFormat() string {
	return r.format
}
