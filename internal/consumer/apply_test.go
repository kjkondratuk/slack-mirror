package consumer

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kjkondratuk/slack-mirror/internal/model"
)

type recordingStore struct {
	upserts  []model.MessageRow
	deletes  [][2]string
	channels []model.Channel
	users    []model.User
}

func (s *recordingStore) UpsertMessage(_ context.Context, m model.MessageRow) error {
	s.upserts = append(s.upserts, m)
	return nil
}
func (s *recordingStore) DeleteMessage(_ context.Context, ch, ts string) error {
	s.deletes = append(s.deletes, [2]string{ch, ts})
	return nil
}
func (s *recordingStore) UpsertChannel(_ context.Context, c model.Channel) error {
	s.channels = append(s.channels, c)
	return nil
}
func (s *recordingStore) UpsertUser(_ context.Context, u model.User) error {
	s.users = append(s.users, u)
	return nil
}
func (s *recordingStore) Close() {}

type recordingResolver struct {
	ensuredChannels []string
	ensuredUsers    []string
}

func (r *recordingResolver) EnsureChannel(_ context.Context, id string) error {
	r.ensuredChannels = append(r.ensuredChannels, id)
	return nil
}
func (r *recordingResolver) EnsureUser(_ context.Context, id string) error {
	r.ensuredUsers = append(r.ensuredUsers, id)
	return nil
}

func TestApplyUpsertEnsuresMetadataFirst(t *testing.T) {
	ctx := context.Background()
	st := &recordingStore{}
	rs := &recordingResolver{}

	row := model.MessageRow{ChannelID: "C1", TS: "1700000000.000100", UserID: "U1",
		Text: "hi", Raw: json.RawMessage(`{}`), PostedAt: time.Unix(1700000000, 0)}
	act := model.Action{Kind: model.ActionUpsert, ChannelID: "C1", TS: row.TS, Message: &row}

	if err := Apply(ctx, st, rs, act); err != nil {
		t.Fatal(err)
	}
	if len(rs.ensuredChannels) != 1 || rs.ensuredChannels[0] != "C1" {
		t.Fatalf("channel not ensured: %v", rs.ensuredChannels)
	}
	if len(rs.ensuredUsers) != 1 || rs.ensuredUsers[0] != "U1" {
		t.Fatalf("user not ensured: %v", rs.ensuredUsers)
	}
	if len(st.upserts) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(st.upserts))
	}
}

func TestApplyDelete(t *testing.T) {
	ctx := context.Background()
	st := &recordingStore{}
	rs := &recordingResolver{}
	act := model.Action{Kind: model.ActionDelete, ChannelID: "C1", TS: "1700000000.000100"}
	if err := Apply(ctx, st, rs, act); err != nil {
		t.Fatal(err)
	}
	if len(st.deletes) != 1 || st.deletes[0] != [2]string{"C1", "1700000000.000100"} {
		t.Fatalf("delete wrong: %v", st.deletes)
	}
}

func TestApplySkipIsNoop(t *testing.T) {
	ctx := context.Background()
	st := &recordingStore{}
	rs := &recordingResolver{}
	if err := Apply(ctx, st, rs, model.Action{Kind: model.ActionSkip}); err != nil {
		t.Fatal(err)
	}
	if len(st.upserts)+len(st.deletes) != 0 {
		t.Fatal("skip should write nothing")
	}
}
