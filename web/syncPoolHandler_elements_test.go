package web

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTwoLevelPath(t *testing.T) {
	assert := assert.New(t)
	paths := TwoLevelPath("1234567")
	if assert.Len(paths, 2) {
		assert.Equal("76", paths[0])
		assert.Equal("54", paths[1])
	}
}

func TestHandlerPoolPathAndFile(t *testing.T) {
	assert := assert.New(t)
	handler := newHandlerPool("/tmp", 1, nil, nil)

	{
		path, filename := (handler.PathAndFile("12345"))
		assert.Equal("/tmp/54/32", path)
		assert.Equal("12345.db", filename)
	}

	{
		path, filename := (handler.PathAndFile("123"))
		assert.Equal("/tmp/32", path)
		assert.Equal("123.db", filename)
	}
}

func TestHandlerPoolGetElement(t *testing.T) {
	assert := assert.New(t)

	tmpdir, err := ioutil.TempDir("", "")
	if !assert.NoError(err) {
		return
	}

	handler := newHandlerPool(tmpdir, 1, nil, nil)
	el, created, err := handler.getElement("123456")
	if assert.NoError(err) {
		assert.NotEmpty(el)
		assert.True(created)
	}
}
