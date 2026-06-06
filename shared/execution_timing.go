package shared

import (
	"foundation/formatting"
	"signal"
	"sort"
	"time"
)

func HelmFormatDuration(duration time.Duration) string {
	return formatting.FormatDurationNSF64(float64(duration.Nanoseconds()))
}

func HelmDurationTotal(durations []time.Duration) time.Duration {
	var total int64
	for _, duration := range durations {
		total += duration.Nanoseconds()
	}
	return time.Duration(total)
}

func HelmDurationMedian(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	nanoseconds := make([]int64, len(durations))
	for index, duration := range durations {
		nanoseconds[index] = duration.Nanoseconds()
	}

	sort.Slice(nanoseconds, func(left, right int) bool {
		return nanoseconds[left] < nanoseconds[right]
	})

	mid := len(nanoseconds) / 2
	if len(nanoseconds)%2 == 1 {
		return time.Duration(nanoseconds[mid])
	}

	return time.Duration((nanoseconds[mid-1] + nanoseconds[mid]) / 2)
}

func HelmDurationAverage(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	return HelmDurationTotal(durations) / time.Duration(len(durations))
}

func helmExecutionRecordDuration(tally *HelmExecutionTally, sig signal.Signal) {
	durationNS, err := signal.SignalPayloadGetAs[int64](&sig, DurationNSPayloadKey)
	if err != nil || durationNS <= 0 {
		return
	}
	tally.RunDurations = append(tally.RunDurations, time.Duration(durationNS))
}
