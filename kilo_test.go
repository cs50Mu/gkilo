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

func TestInsertSlice2(t *testing.T) {
	s := []rune{'a', 'b', 'c'}
	printSlice(s)
	Insert(&s, 2, 'd')
	printSlice(s)
}

// Insert 在切片的指定位置插入一个元素
func Insert(slice *[]rune, index int, value rune) {
	// 确保切片可以容纳新元素
	*slice = append(*slice, 0)                 // 在切片末尾追加一个零值
	copy((*slice)[index+1:], (*slice)[index:]) // 移动元素
	(*slice)[index] = value                    // 在指定位置插入新元素
}

func CxToRx(s string, cx int) int {
	var rx int
	for i := 0; i < cx; i++ {
		if s[i] == '\t' {
			rx += KILO_TAB_STOP
		} else {
			rx += 1
		}
	}

	return rx
}

func TestCxToRx(t *testing.T) {
	s := "h\t\tello"
	cx := 2
	fmt.Printf("cx: %v, rx: %v\n", cx, CxToRx(s, cx))
}

func TestPunct(t *testing.T) {
	c := '。'
	fmt.Printf("%c: %v\n", c, isSeparator(c))
}
