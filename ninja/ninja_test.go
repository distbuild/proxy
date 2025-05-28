package ninja

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	testNinjaName = "../test/build.ninja"
)

func initNinjaTest() ninja {
	return ninja{
		file: testNinjaName,
	}
}

func TestCheck(t *testing.T) {
	ctx := context.Background()
	n := initNinjaTest()

	err := n.check(ctx)
	assert.Equal(t, nil, err)
}

func TestRun(t *testing.T) {
	ctx := context.Background()
	n := initNinjaTest()

	_, err := n.run(ctx)
	assert.Equal(t, nil, err)
}

func TestParse(t *testing.T) {
	ctx := context.Background()
	n := initNinjaTest()

	buf, _ := n.run(ctx)

	_, err := n.parse(ctx, buf)
	assert.Equal(t, nil, err)
}
