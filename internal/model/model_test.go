package model

import "testing"

func TestActionKindString(t *testing.T) {
	cases := map[ActionKind]string{
		ActionSkip:   "skip",
		ActionUpsert: "upsert",
		ActionDelete: "delete",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Fatalf("ActionKind(%d).String() = %q, want %q", k, got, want)
		}
	}
}
