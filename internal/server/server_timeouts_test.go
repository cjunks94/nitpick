package server

import (
	"net/http"
	"testing"
)

// GH-2: ReadHeaderTimeout alone leaves the request body unbounded in time,
// so a client trickling bytes holds a goroutine open indefinitely. Every
// timeout must be set, and none may be looser than the header timeout.
func TestNewHTTPServer_SetsEveryTimeout(t *testing.T) {
	srv := newHTTPServer(":0", http.NotFoundHandler())

	if srv.ReadHeaderTimeout <= 0 {
		t.Fatal("ReadHeaderTimeout unset")
	}
	if srv.ReadTimeout <= 0 {
		t.Fatal("ReadTimeout unset: slow body is a slowloris vector")
	}
	if srv.WriteTimeout <= 0 {
		t.Fatal("WriteTimeout unset")
	}
	if srv.IdleTimeout <= 0 {
		t.Fatal("IdleTimeout unset: keep-alive connections never reclaimed")
	}
	if srv.ReadTimeout < srv.ReadHeaderTimeout {
		t.Fatalf("ReadTimeout %v shorter than ReadHeaderTimeout %v; the outer cap would fire before headers finish",
			srv.ReadTimeout, srv.ReadHeaderTimeout)
	}
	if srv.Addr != ":0" {
		t.Fatalf("Addr = %q, want :0", srv.Addr)
	}
}
