package unit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	chatdomain "github.com/Ammar022/muhammad-ammar-azam/internal/chat/domain"
)

func TestFreeMessagesPerMonth(t *testing.T) {
	assert.Equal(t, 3, chatdomain.FreeMessagesPerMonth)
}

func TestCurrentMonth(t *testing.T) {
	month := chatdomain.CurrentMonth()
	assert.Len(t, month, 7)
	assert.Equal(t, string(month[4]), "-")

	expected := time.Now().UTC().Format("2006-01")
	assert.Equal(t, expected, month)
}

// TestQuotaUsage_FreeQuotaNotExhausted checks the < 3 boundary.
func TestQuotaUsage_FreeQuotaNotExhausted(t *testing.T) {
	cases := []struct {
		used      int
		exhausted bool
	}{
		{0, false},
		{1, false},
		{2, false},
		{3, true}, // exactly at limit → exhausted
		{4, true}, // over limit → exhausted
	}
	for _, tc := range cases {
		isExhausted := tc.used >= chatdomain.FreeMessagesPerMonth
		assert.Equal(t, tc.exhausted, isExhausted,
			"used=%d: expected exhausted=%v", tc.used, tc.exhausted)
	}
}
