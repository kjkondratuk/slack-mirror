// Package model holds the leaf data types shared across the service. It has no
// dependencies on other internal packages to keep the import graph acyclic.
package model

import "time"

type ActionKind int

const (
	ActionSkip ActionKind = iota
	ActionUpsert
	ActionDelete
)

func (k ActionKind) String() string {
	switch k {
	case ActionUpsert:
		return "upsert"
	case ActionDelete:
		return "delete"
	default:
		return "skip"
	}
}

// Action is the result of dispatching a single Slack message event.
type Action struct {
	Kind      ActionKind
	ChannelID string
	TS        string      // message ts (the storage key within a channel)
	Message   *MessageRow // non-nil only when Kind == ActionUpsert
}

// MessageRow is one row of the current-state mirror.
type MessageRow struct {
	ChannelID string
	TS        string
	ThreadTS  string // empty if not a thread reply
	UserID    string // empty for some system/bot messages
	Text      string
	Subtype   string     // empty for normal messages
	Raw       []byte     // latest full message payload (JSON)
	PostedAt  time.Time  // derived from TS
	EditedAt  *time.Time // from message.edited.ts when present
}

type Channel struct {
	ID        string
	Name      string
	IsPrivate bool
}

type User struct {
	ID       string
	Username string
	RealName string
	IsBot    bool
}

// FileRef is the metadata extracted from a message's files[] entry.
type FileRef struct {
	ID          string
	Name        string
	Title       string
	Mimetype    string
	Filetype    string
	Size        int64
	UserID      string
	Mode        string // hosted|external|snippet|…
	IsExternal  bool
	URLDownload string // url_private_download
	Raw         []byte
}

// FileRow is the persisted file record (FileRef + storage state).
type FileRow struct {
	FileRef
	StorageURI    string
	SHA256        string
	DownloadState string // pending|stored|skipped|failed
}

// SearchHit is one full-text search result.
type SearchHit struct {
	ChannelID string
	TS        string
	Text      string
}
