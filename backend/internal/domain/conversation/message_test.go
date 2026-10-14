package conversation

import (
	"reflect"
	"testing"
)

func TestCollectMessageRunIDsTrimsAndDeduplicates(t *testing.T) {
	messages := []Message{
		{RunID: " run_1 "},
		{RunID: "run_1"},
		{RunID: ""},
		{RunID: "run_2"},
		{RunID: "run_2"},
	}

	got := CollectMessageRunIDs(messages)
	want := []string{"run_1", "run_2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected run ids: got %#v want %#v", got, want)
	}
}

func TestCollectMessageRunIDsSkipsEmpty(t *testing.T) {
	messages := []Message{
		{RunID: ""},
		{RunID: "  "},
	}

	if got := CollectMessageRunIDs(messages); len(got) != 0 {
		t.Fatalf("expected 0 run IDs for empty inputs, got %d", len(got))
	}
}
