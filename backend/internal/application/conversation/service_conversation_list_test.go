package conversation

import (
	"testing"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/pagination"
)

func TestNormalizeRecentMessageLimitUsesMessageWindow(t *testing.T) {
	if got := normalizeRecentMessageLimit(5000); got != pagination.MaxPageSize {
		t.Fatalf("expected recent message limit capped at %d, got %d", pagination.MaxPageSize, got)
	}
}
