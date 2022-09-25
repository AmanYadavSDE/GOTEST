package bot

import (
	"gotest.tools/v3/assert"
	"testing"
)

func TestAddition(t *testing.T) {

	got := Addition(1, 3)

	want := 4
	assert.Equal(t, got, want)

}
