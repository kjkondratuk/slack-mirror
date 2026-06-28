package sqlitestore

import (
	"context"
	"fmt"

	"github.com/kjkondratuk/slack-mirror/internal/model"
)

// Search runs a BM25-ranked FTS5 query, most-relevant first.
func (s *Store) Search(ctx context.Context, query string, limit int) ([]model.SearchHit, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.channel_id, m.ts, m.text
		FROM messages_fts f JOIN messages m ON m.rowid = f.rowid
		WHERE messages_fts MATCH ?
		ORDER BY rank
		LIMIT ?`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}
	defer rows.Close()
	var hits []model.SearchHit
	for rows.Next() {
		var h model.SearchHit
		if err := rows.Scan(&h.ChannelID, &h.TS, &h.Text); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}
