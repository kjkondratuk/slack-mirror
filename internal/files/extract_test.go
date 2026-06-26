package files

import (
	"testing"

	"github.com/slack-go/slack"
)

func TestFromMsg(t *testing.T) {
	m := &slack.Msg{
		Files: []slack.File{
			{ID: "F1", Name: "a.png", Title: "A", Mimetype: "image/png", Filetype: "png",
				Size: 1234, User: "U1", URLPrivateDownload: "https://files.slack.com/a.png", Mode: "hosted"},
			{ID: "F2", Name: "ext", Mode: "external", IsExternal: true},
		},
	}
	refs := FromMsg(m)
	if len(refs) != 2 {
		t.Fatalf("got %d refs", len(refs))
	}
	if refs[0].ID != "F1" || refs[0].Mimetype != "image/png" || refs[0].Size != 1234 {
		t.Fatalf("bad ref0: %+v", refs[0])
	}
	if refs[0].URLDownload != "https://files.slack.com/a.png" {
		t.Fatalf("download url = %q", refs[0].URLDownload)
	}
	if len(refs[0].Raw) == 0 || refs[0].Raw[0] != '{' {
		t.Fatalf("raw should be JSON object, got %q", refs[0].Raw)
	}
	if !refs[1].IsExternal {
		t.Fatalf("F2 should be external")
	}
}

func TestFromMsgNoFiles(t *testing.T) {
	if refs := FromMsg(&slack.Msg{}); len(refs) != 0 {
		t.Fatalf("expected no refs, got %d", len(refs))
	}
}
