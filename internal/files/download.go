package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"

	"github.com/kjkondratuk/slack-mirror/internal/blobstore"
	"github.com/kjkondratuk/slack-mirror/internal/model"
)

// FileStore is the persistence surface the downloader writes to.
type FileStore interface {
	UpsertFile(ctx context.Context, r model.FileRow) error
	LinkFile(ctx context.Context, channelID, messageTS, fileID string) error
	SetFileStored(ctx context.Context, id, storageURI, sha256 string) error
	SetFileState(ctx context.Context, id, state string) error
}

type Downloader struct {
	HTTP      *http.Client
	Token     string
	Blobs     blobstore.Blobstore
	Store     FileStore
	MaxBytes  int64
	MimeAllow map[string]bool
}

// Handle records each file ref, links it to the message, and downloads bytes
// unless filtered. Errors on individual files are recorded as state=failed and
// do not abort the batch.
func (d *Downloader) Handle(ctx context.Context, channelID, messageTS string, refs []model.FileRef) error {
	for _, ref := range refs {
		row := model.FileRow{FileRef: ref, DownloadState: "pending"}
		if err := d.Store.UpsertFile(ctx, row); err != nil {
			return err
		}
		if err := d.Store.LinkFile(ctx, channelID, messageTS, ref.ID); err != nil {
			return err
		}
		if d.skip(ref) {
			_ = d.Store.SetFileState(ctx, ref.ID, "skipped")
			continue
		}
		if err := d.download(ctx, ref); err != nil {
			_ = d.Store.SetFileState(ctx, ref.ID, "failed")
		}
	}
	return nil
}

func (d *Downloader) skip(ref model.FileRef) bool {
	if ref.IsExternal || ref.URLDownload == "" {
		return true
	}
	if d.MaxBytes > 0 && ref.Size > d.MaxBytes {
		return true
	}
	if len(d.MimeAllow) > 0 && !d.MimeAllow[ref.Mimetype] {
		return true
	}
	return false
}

func (d *Downloader) download(ctx context.Context, ref model.FileRef) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref.URLDownload, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.Token)
	resp, err := d.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: status %d", ref.ID, resp.StatusCode)
	}

	hasher := sha256.New()
	tee := io.TeeReader(resp.Body, hasher)
	uri, err := d.Blobs.Put(ctx, "files/"+ref.ID, tee, ref.Mimetype)
	if err != nil {
		return err
	}
	return d.Store.SetFileStored(ctx, ref.ID, uri, hex.EncodeToString(hasher.Sum(nil)))
}
