package subscriptions

import (
	"context"
	"errors"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestResolveChannelIDSuccess(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(req.URL.Path, "/@handle/about") {
			t.Fatalf("unexpected path %s", req.URL.Path)
		}
		body := `{"channelId":"UC1234567890123456789012"}`
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       ioutil.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    req,
		}
		return resp, nil
	})}

	id, err := ResolveChannelID(context.Background(), "handle", client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "UC1234567890123456789012" {
		t.Fatalf("unexpected id %q", id)
	}
}

func TestResolveChannelIDValidatesInput(t *testing.T) {
	if _, err := ResolveChannelID(context.Background(), "", nil); err == nil {
		t.Fatalf("expected error for empty handle")
	}
}

func TestResolveChannelIDPropagatesErrors(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("boom")
	})}
	if _, err := ResolveChannelID(context.Background(), "@handle", client); err == nil {
		t.Fatalf("expected error when transport fails")
	}
}

func TestResolveChannelIDStatusCheck(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := &http.Response{StatusCode: http.StatusNotFound, Body: ioutil.NopCloser(strings.NewReader("")), Header: make(http.Header)}
		return resp, nil
	})}
	if _, err := ResolveChannelID(context.Background(), "@handle", client); err == nil {
		t.Fatalf("expected error on non-200 status")
	}
}

func TestResolveChannelIDRequiresMatch(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := &http.Response{StatusCode: http.StatusOK, Body: ioutil.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}
		return resp, nil
	})}
	if _, err := ResolveChannelID(context.Background(), "@handle", client); err == nil {
		t.Fatalf("expected error when pattern missing")
	}
}
