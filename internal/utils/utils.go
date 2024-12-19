package utils

import (
	"fmt"
	"net/url"

	"github.com/dustin/go-humanize"
)

// HumanizeBytes converts bytes to a human readable format
func HumanizeBytes(b int64, rate bool) string {
	var suffix string

	if rate {
		suffix = "/s"
	}

	return fmt.Sprintf("%s%s",
		humanize.Bytes(uint64(b)),
		suffix)
}

// HumanizeDurationSeconds returns human readable duration for value in float64 seconds
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

// ValidateUrl validates URL
func ValidateUrl(inURL string) error {

	u, err := url.Parse(inURL)

	if err != nil {
		return err
	}

	if u.Scheme == "" {
		return fmt.Errorf("can't find scheme in URL %q", inURL)
	}

	if u.Host == "" {
		return fmt.Errorf("can't find host in URL %q", inURL)
	}

	return nil
}

// ParseS3URL splits s3 URL into bucket and key
func ParseS3URL(uri string) (string, string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", "", err
	}

	return u.Host, u.Path, nil
}
