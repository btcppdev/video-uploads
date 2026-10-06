package catalog

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestConferencesPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/conferences" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("cursor") == "" {
			fmt.Fprint(w, `{"data":[{"id":"old","tag":"old","starts_at":"2025-01-01"}],"meta":{"next_cursor":"next+page"}}`)
		} else {
			if r.URL.Query().Get("cursor") != "next+page" {
				t.Error("cursor was not escaped")
			}
			fmt.Fprint(w, `{"data":[{"id":"new","tag":"new","starts_at":"2026-01-01"}]}`)
		}
	}))
	defer server.Close()
	for _, base := range []string{server.URL, server.URL + "/api/v1/"} {
		confs, err := New(base).Conferences(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(confs) != 2 || confs[0].ID != "new" || confs[1].ID != "old" {
			t.Fatalf("unexpected conferences: %+v", confs)
		}
	}
}

func TestUploadContext(t *testing.T) {
	for _, test := range []struct {
		name, body            string
		status                int
		hasHackathon, warning bool
	}{
		{"published", `{"data":[{"id":"hack"}]}`, 200, true, false},
		{"none", `{"data":[]}`, 200, false, false},
		{"unavailable", ``, 503, false, true},
		{"malformed", `<html>error</html>`, 200, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/conferences/toronto/days":
					fmt.Fprint(w, `{"data":[{"day_number":2,"venues":null},{"day_number":1,"venues":["one","two","one"," "]}]}`)
				case "/api/v1/conferences/toronto/hackathons":
					w.WriteHeader(test.status)
					fmt.Fprint(w, test.body)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			got, err := New(server.URL).UploadContext(context.Background(), "toronto")
			if err != nil {
				t.Fatal(err)
			}
			if got.HasHackathon != test.hasHackathon || (got.HackathonError != "") != test.warning {
				t.Fatalf("unexpected context: %+v", got)
			}
			if len(got.Days) != 2 || got.Days[0].Number != 1 || !reflect.DeepEqual(got.Days[0].Rooms, []string{"one", "two"}) || got.Days[1].Rooms == nil {
				t.Fatalf("unexpected days: %+v", got.Days)
			}
		})
	}
}

func TestAPIErrors(t *testing.T) {
	for _, body := range []string{`{"error":{"message":"no access"}}`, `<html>unavailable</html>`, `{"data":{}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		_, err := New(server.URL).Conferences(context.Background())
		server.Close()
		if err == nil {
			t.Fatalf("expected error for %s", body)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	if _, err := New(server.URL).UploadContext(context.Background(), "event"); err == nil {
		t.Fatal("expected days error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New(server.URL).Conferences(ctx); err == nil {
		t.Fatal("expected cancellation")
	}
}

func TestRepeatedCursor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[],"meta":{"next_cursor":"same"}}`)
	}))
	defer server.Close()
	if _, err := New(server.URL).Conferences(context.Background()); err == nil {
		t.Fatal("expected repeated cursor error")
	}
}
