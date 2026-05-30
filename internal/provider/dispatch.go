package provider

import (
	"bytes"
	"context"
	"io"
	"net/http"
)

// Dispatch is the thin, router-facing entrypoint that is INTENDED to replace
// the format-switch inside the router's upstream call. It is provided in the
// foundation commit so the integration contract is concrete and testable, but
// it is NOT called from any live path in this commit.
//
// The design decision that makes the whole SDK work: the router injects how to
// perform the HTTP call (httpDo) and how to read the body (readAll). Dispatch
// asks the provider to Prepare the request, performs the call via the injected
// httpDo, then asks the provider to Translate the response. Because httpDo is
// injected, the router can wrap it with circuit/retry/fallback/hedge — the
// reliability layer stays OUTSIDE the provider (decorator model), and shared
// response post-processing runs in the router AFTER Translate.
func Dispatch(
	ctx context.Context,
	p Provider,
	req *Request,
	conn *ConnConfig,
	httpDo func(*http.Request) (*http.Response, error),
	readAll func(*http.Response) ([]byte, error),
) (*Response, error) {
	// 1. Provider builds the upstream request (replaces the format-switch).
	up, err := p.Prepare(ctx, req, conn)
	if err != nil {
		return nil, err
	}

	// 2. Router owns the HTTP call so reliability can wrap it. Dispatch only
	//    assembles the *http.Request; execution is the injected httpDo.
	httpReq, err := http.NewRequestWithContext(ctx, up.Method, up.URL, bytes.NewReader(up.Body))
	if err != nil {
		return nil, err
	}
	for k, vs := range up.Header {
		for _, v := range vs {
			httpReq.Header.Add(k, v)
		}
	}

	resp, err := httpDo(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if readAll == nil {
		readAll = func(r *http.Response) ([]byte, error) { return io.ReadAll(r.Body) }
	}
	raw, err := readAll(resp)
	if err != nil {
		return nil, err
	}

	// 3. Provider translates native → canonical.
	out, err := p.Translate(ctx, raw, req)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = &Response{}
	}
	out.Status = resp.StatusCode
	// 4. (In the live router) shared post-processing runs here on out.Body.
	return out, nil
}
