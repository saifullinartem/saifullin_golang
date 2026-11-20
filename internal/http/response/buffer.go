package response

import (
	"bytes"
	"fmt"
	"net/http"
)

type Buffer struct {
	header   http.Header
	snapshot http.Header
	body     bytes.Buffer
	status   int
}

func NewBuffer() *Buffer {
	return &Buffer{header: make(http.Header)}
}

func (b *Buffer) Header() http.Header {
	return b.header
}

func (b *Buffer) WriteHeader(status int) {
	if b.status != 0 {
		return
	}
	if status < 100 || status > 999 {
		panic(fmt.Sprintf("invalid WriteHeader code %d", status))
	}
	if status < 200 {
		return
	}
	b.status = status
	b.snapshot = b.header.Clone()
}

func (b *Buffer) Write(data []byte) (int, error) {
	if b.status == 0 {
		b.WriteHeader(http.StatusOK)
	}
	return b.body.Write(data)
}

func (b *Buffer) Reset() {
	clear(b.header)
	b.body.Reset()
	b.status = 0
	b.snapshot = nil
}

func (b *Buffer) Commit(w http.ResponseWriter) {
	if b.status == 0 {
		b.WriteHeader(http.StatusOK)
	}
	for key, values := range b.snapshot {
		w.Header()[key] = append([]string(nil), values...)
	}
	w.WriteHeader(b.status)
	_, _ = w.Write(b.body.Bytes())
}
