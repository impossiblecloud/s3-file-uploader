package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateUrl(t *testing.T) {
	assert.Nil(t, validateUrl("s3://my-bucket/"))
	assert.Nil(t, validateUrl("s3://my-bucket/path/to/dir"))
	assert.NotNil(t, validateUrl("s3my-bucket/"))
}
