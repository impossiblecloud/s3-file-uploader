package utils

import (
	"fmt"

	"github.com/dustin/go-humanize"
)

// Convert bytes to a human readable format
func HumanizeBytes(b int64, rate bool) string {
	var suffix string

	if rate {
		suffix = "/s"
	}

	return fmt.Sprintf("%s%s",
		humanize.Bytes(uint64(b)),
		suffix)
}

// Returns human readable duration for value in float64 seconds
func HumanizeDurationSeconds(seconds float64) string {
	const unit = 1000
	const nanosecondsInSecond = 1e+9

	duration := seconds * nanosecondsInSecond

	if duration < unit {
		return fmt.Sprintf("%.2fns", duration)
	}

	div, exp := int64(unit), 0
	units := [3]string{"µs", "ms", "s"}

	for n := duration / unit; n >= unit; n /= unit {
		div *= unit
		exp++
		if exp >= len(units)-1 {
			break
		}
	}

	return fmt.Sprintf("%.2f%s",
		float64(duration)/float64(div), units[exp])
}
