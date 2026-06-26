// Package files extracts file metadata from Slack messages and downloads the
// bytes into a Blobstore when enabled.
package files

import (
	"encoding/json"

	"github.com/kjkondratuk/slack-mirror/internal/model"
	"github.com/slack-go/slack"
)

// FromMsg extracts file metadata from a slack.Msg's files[]. Works for backfilled
// history (slack.Message embeds Msg), the inner message of a message_changed event,
// and a live message parsed from the raw payload.
func FromMsg(m *slack.Msg) []model.FileRef {
	out := make([]model.FileRef, 0, len(m.Files))
	for i := range m.Files {
		f := m.Files[i]
		raw, _ := json.Marshal(f)
		out = append(out, model.FileRef{
			ID: f.ID, Name: f.Name, Title: f.Title,
			Mimetype: f.Mimetype, Filetype: f.Filetype,
			Size: int64(f.Size), UserID: f.User,
			Mode: f.Mode, IsExternal: f.IsExternal,
			URLDownload: f.URLPrivateDownload, Raw: raw,
		})
	}
	return out
}
