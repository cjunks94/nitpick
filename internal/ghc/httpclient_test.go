package ghc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchFile_404WrapsErrFileNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer srv.Close()
	client := &HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	_, err := client.FetchFile(context.Background(), "owner/repo", "abc", ".nitpick.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrFileNotFound) {
		t.Errorf("expected ErrFileNotFound wrapping, got %v", err)
	}
}

func TestFetchFile_500DoesNotWrapErrFileNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream exploded"))
	}))
	defer srv.Close()
	client := &HTTPClient{BaseURL: srv.URL, Token: "test", HTTPClient: srv.Client()}

	_, err := client.FetchFile(context.Background(), "owner/repo", "abc", ".nitpick.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, ErrFileNotFound) {
		t.Errorf("500 should not match ErrFileNotFound, but errors.Is returned true: %v", err)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to mention status 500, got: %v", err)
	}
}
