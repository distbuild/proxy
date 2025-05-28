package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateBuildID(t *testing.T) {
	_, err := createBuildID()
	assert.Equal(t, nil, err)
}

func TestGetTargetPath(t *testing.T) {
	request := []string{
		"/path/to/name1",
		"/path/to/name2",
	}

	target := "/path/to/name1"

	_path, err := getTargetPath(request, target)
	assert.Equal(t, nil, err)
	assert.Equal(t, request[0], _path)
}
