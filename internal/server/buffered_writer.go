package server

import (
	"bytes"
	"net/http"
)

// bufferedResponseWriter captures a handler's status, headers, and body in
// memory instead of writing them to the network. It exists for the M6
// non-streaming Responses path, where the chat handler's JSON body must be
// translated into the Responses shape BEFORE anything reaches the client.
//
// It deliberately does NOT implement http.Flusher: the chat handler probes for
// Flusher to decide whether it may stream. Withholding it keeps the handler on
// its buffered branch, which is exactly what the non-streaming path wants.
// (The streaming path uses responsesStreamAdapter, which does implement it.)
type bufferedResponseWriter struct {
	header      http.Header
	status      int
	body        bytes.Buffer
	wroteHeader bool
}

func (b *bufferedResponseWriter) Header() http.Header { return b.header }

func (b *bufferedResponseWriter) WriteHeader(status int) {
	if b.wroteHeader {
		return
	}
	b.wroteHeader = true
	b.status = status
}

func (b *bufferedResponseWriter) Write(p []byte) (int, error) {
	if !b.wroteHeader {
		b.WriteHeader(http.StatusOK)
	}
	return b.body.Write(p)
}
