package main

import (
	"fmt"
	"slices"
	"testing"
)

func testCxToRx(t *testing.T) {
	erow := &editorRow{
		size:        0,
		rawChars:    []rune("this\tis\ta\ttest"),
		rsize:       0,
		renderChars: []rune{},
	}

	cx := 7
	t.Logf("cx: %v, rx: %v\n", cx, editorRowCxToRx(erow, cx))
}

func insertEle(slice *[]rune, at int, ch rune) {
	*slice = append((*slice)[:at], append([]rune{ch}, (*slice)[at:]...)...)
}

func TestInsertChar(t *testing.T) {
	s := []rune{'a', 'b', 'c'}
	printSlice(s)
	insertEle(&s, 1, 'x')
	printSlice(s)
	insertEle(&s, 2, 'y')
	printSlice(s)
}

func printSlice(slice []rune) {
	fmt.Printf("%c\n", slice)
}

func TestInsertSlice(t *testing.T) {
	s := []rune{'a', 'b', 'c'}
	printSlice(s)
	x := slices.Insert(s, 1, s[1:]...)
	printSlice(x)
}

func TestDelEleSlice(t *testing.T) {
	s := []rune{'a', 'b', 'c'}
	printSlice(s)
	n := copy(s[1:], s[2:])
	fmt.Printf("copied: %d\n", n)
	s = s[:len(s)-1]
	printSlice(s)
}

func TestRuneString(t *testing.T) {
	a := "hello"
	fmt.Printf("sLen: %v, runeLen: %v\n", len(a), len([]rune(a)))
	a = "你好"
	fmt.Printf("sLen: %v, runeLen: %v\n", len(a), len([]rune(a)))
}
