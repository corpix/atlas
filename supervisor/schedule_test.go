package supervisor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestScheduleNext(t *testing.T) {
	epoch := time.Unix(0, 0).UTC()
	testCases := []struct {
		name     string
		from     time.Time
		expected time.Time
		schedule Schedule
	}{
		{name: "hourly from epoch", schedule: Schedule{Period: time.Hour}, from: epoch, expected: epoch.Add(time.Hour)},
		{name: "strict boundary", schedule: Schedule{Period: time.Hour}, from: epoch.Add(time.Hour), expected: epoch.Add(2 * time.Hour)},
		{name: "offset", schedule: Schedule{Period: 24 * time.Hour, Offset: 3 * time.Hour}, from: epoch, expected: epoch.Add(3 * time.Hour)},
		{name: "offset wraps", schedule: Schedule{Period: 24 * time.Hour, Offset: 3 * time.Hour}, from: epoch.Add(4 * time.Hour), expected: epoch.Add(27 * time.Hour)},
		{name: "before epoch", schedule: Schedule{Period: time.Hour}, from: epoch.Add(-90 * time.Minute), expected: epoch.Add(-time.Hour)},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.schedule.Next(tc.from))
		})
	}
}

func TestScheduleValidate(t *testing.T) {
	testCases := []struct {
		expected error
		schedule Schedule
	}{
		{schedule: Schedule{}, expected: ErrPeriodInvalid},
		{schedule: Schedule{Period: -time.Hour}, expected: ErrPeriodInvalid},
		{schedule: Schedule{Period: time.Hour, Offset: -time.Minute}, expected: ErrOffsetInvalid},
		{schedule: Schedule{Period: time.Hour, Offset: time.Hour}, expected: ErrOffsetInvalid},
	}
	for _, tc := range testCases {
		assert.ErrorIs(t, tc.schedule.Validate(), tc.expected)
	}
	assert.NoError(t, (Schedule{Period: time.Hour, Offset: time.Minute}).Validate())
}
