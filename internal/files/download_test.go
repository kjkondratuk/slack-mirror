package files

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kjkondratuk/slack-mirror/internal/model"
)

type rt func(*http.Request) (*http.Response, error)

func (f rt) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type fakeBlobs struct {
	puts    map[string]string
	deletes []string
}

func (b *fakeBlobs) Put(_ context.Context, key string, r io.Reader, _ string) (string, error) {
	data, _ := io.ReadAll(r)
	if b.puts == nil {
		b.puts = map[string]string{}
	}
	uri := "mem://" + key
	b.puts[uri] = string(data)
	return uri, nil
}
func (b *fakeBlobs) Delete(_ context.Context, uri string) error {
	b.deletes = append(b.deletes, uri)
	return nil
}

type fakeFileStore struct {
	rows   map[string]model.FileRow
	links  [][3]string
	stored map[string][2]string // id -> {uri, sha}
	states map[string]string
}

func newFakeFileStore() *fakeFileStore {
	return &fakeFileStore{rows: map[string]model.FileRow{}, stored: map[string][2]string{}, states: map[string]string{}}
}
func (s *fakeFileStore) UpsertFile(_ context.Context, r model.FileRow) error { s.rows[r.ID] = r; return nil }
func (s *fakeFileStore) LinkFile(_ context.Context, ch, ts, id string) error {
	s.links = append(s.links, [3]string{ch, ts, id})
	return nil
}
func (s *fakeFileStore) SetFileStored(_ context.Context, id, uri, sha string) error {
	s.stored[id] = [2]string{uri, sha}
	return nil
}
func (s *fakeFileStore) SetFileState(_ context.Context, id, state string) error {
	s.states[id] = state
	return nil
}

func newDownloader(blobs *fakeBlobs, st *fakeFileStore, maxBytes int64, mime map[string]bool) *Downloader {
	hc := &http.Client{Transport: rt(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer xoxb-test" {
			return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("PNGDATA"))}, nil
	})}
	return &Downloader{HTTP: hc, Token: "xoxb-test", Blobs: blobs, Store: st, MaxBytes: maxBytes, MimeAllow: mime}
}

func TestDownloadStoresFile(t *testing.T) {
	ctx := context.Background()
	blobs := &fakeBlobs{}
	st := newFakeFileStore()
	d := newDownloader(blobs, st, 0, nil)

	refs := []model.FileRef{{ID: "F1", Mimetype: "image/png", Size: 7, URLDownload: "https://x/a.png"}}
	if err := d.Handle(ctx, "C1", "1700000000.000100", refs); err != nil {
		t.Fatal(err)
	}
	if st.stored["F1"][0] != "mem://files/F1" {
		t.Fatalf("stored uri = %v", st.stored["F1"])
	}
	if blobs.puts["mem://files/F1"] != "PNGDATA" {
		t.Fatalf("blob content = %q", blobs.puts["mem://files/F1"])
	}
	if len(st.links) != 1 || st.links[0] != [3]string{"C1", "1700000000.000100", "F1"} {
		t.Fatalf("link wrong: %v", st.links)
	}
}

func TestDownloadSkipsExternalOversizeAndMime(t *testing.T) {
	ctx := context.Background()
	st := newFakeFileStore()
	d := newDownloader(&fakeBlobs{}, st, 100, map[string]bool{"image/png": true})

	refs := []model.FileRef{
		{ID: "Fext", IsExternal: true, URLDownload: "https://x"},
		{ID: "Fbig", Mimetype: "image/png", Size: 1000, URLDownload: "https://x"},
		{ID: "Fmime", Mimetype: "application/zip", Size: 10, URLDownload: "https://x"},
	}
	if err := d.Handle(ctx, "C1", "1700000000.000100", refs); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"Fext", "Fbig", "Fmime"} {
		if st.states[id] != "skipped" {
			t.Fatalf("%s state = %q, want skipped", id, st.states[id])
		}
	}
}
