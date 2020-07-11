package imageresizer

import (
	"testing"
)

func TestTemplate(t *testing.T) {

	template := NewTemplate("custom-w200")

	if template.width != 200 {
		t.Error("With failed")
	}

	if template.height != 0 {
		t.Error("Height failed")
	}
}
