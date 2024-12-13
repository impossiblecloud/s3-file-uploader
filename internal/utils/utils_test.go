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
