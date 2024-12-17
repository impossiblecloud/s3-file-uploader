package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHumanizeDurationSeconds(t *testing.T) {
	requestTime := HumanizeDurationSeconds(6000)
	assert.Equal(t, requestTime, "6000.00s", "they should be equal")

	requestTime = HumanizeDurationSeconds(0.05)
	assert.Equal(t, requestTime, "50.00ms", "they should be equal")

	requestTime = HumanizeDurationSeconds(99999999)
	assert.Equal(t, requestTime, "99999999.00s", "they should be equal")
}

func TestHumanizeBytes(t *testing.T) {
	res := HumanizeBytes(29, false)
	assert.Equal(t, res, "29 B", "they should be equal")

	res = HumanizeBytes(25872882, false)
	assert.Equal(t, res, "26 MB", "they should be equal")
}
