package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestControlCallDoesNotFollowRedirectsOrLeakToken(t *testing.T) {
	var leaked bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer other.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer private" {
			t.Error("missing authentication")
		}
		http.Redirect(w, r, other.URL, http.StatusFound)
	}))
	defer s.Close()
	if _, err := controlCall(control{URL: s.URL, Token: "private"}, "GET", "/status", nil, time.Second); err == nil {
		t.Fatal("accepted redirect")
	}
	if leaked {
		t.Fatal("followed redirect to another origin")
	}
}
