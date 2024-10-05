package main

import "testing"

func TestCxToRx(t *testing.T) {
	erow := &editorRow{
		size:        0,
		rawChars:    []rune("this\tis\ta\ttest"),
		rsize:       0,
		renderChars: []rune{},
	}

	cx := 7
	t.Logf("cx: %v, rx: %v\n", cx, editorRowCxToRx(erow, cx))
}
