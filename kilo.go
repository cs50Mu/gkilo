package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/nsf/termbox-go"
)

const (
	GKILO_VERSION      = "0.0.1"
	ColDef             = termbox.ColorDefault
	ColWhi             = termbox.ColorWhite
	LogFile            = "kilo.log"
	KILO_TAB_STOP      = 4
	FILENAME_MAX_PRINT = 20
	KILO_QUIT_TIMES    = 3
)

type editorRow struct {
	size        int
	rawChars    []rune // 原始的字符集合
	rsize       int
	renderChars []rune // 需要渲染的字符集合
}

type editorConf struct {
	screenRows      int
	screenCols      int
	statusBarRowIdx int
	msgBarRowIdx    int
	cursorX         int
	renderCursorX   int // index into the renderChars field
	cursorY         int
	rows            []*editorRow
	numRows         int
	rowOffset       int
	colOffset       int
	filename        string
	statusMsg       string
	statusMsgTime   time.Time // the timestamp when we set a statusMsg
	modified        bool
}

var E *editorConf
var logger *log.Logger

func main() {
	fileNamePtr := flag.String("f", "", "file to open")

	flag.Parse()

	logfile, err := os.OpenFile(LogFile, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}

	logger = log.New(logfile, "[kilo] ", log.LstdFlags)

	err = termbox.Init()
	if err != nil {
		panic(err)
	}
	defer termbox.Close()

	termbox.SetInputMode(termbox.InputEsc)

	initEditor()

	if *fileNamePtr != "" {
		editorOpen(*fileNamePtr)
	}

	editorSetStatusMsg("HELP: C-S = save | C-Q = quit")

	editorRefreshScreen()

	editorProcessKeypress()
}

// editorProcessKeypress ...
func editorProcessKeypress() {
	kiloQuitTimes := KILO_QUIT_TIMES
loop:
	for {
		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			switch ev.Key {
			case termbox.KeyCtrlD:
				kiloQuitTimes--
				if E.modified && kiloQuitTimes > 0 {
					editorSetStatusMsg(fmt.Sprintf("WARNING!!! File has unsaved changes. Press Ctrl-D %d more times to quit.", kiloQuitTimes))
				} else {
					break loop
				}
			case termbox.KeyEsc:
				break loop
			case termbox.KeyEnter:
				editorInsertNewline()
			// case termbox.KeyCtrlL:

			// Backspace delete the character to the left of the cursor
			// Del delete the character under the cursor
			case termbox.KeyBackspace2, termbox.KeyDelete:
				if ev.Key == termbox.KeyDelete {
					editorMoveCursor('l')
				}
				editorDelChar()
			// case termbox.KeyDelete:
			case termbox.KeyCtrlL:
				editorDelRow(E.cursorY)
			case termbox.KeyCtrlS:
				editorSave()
			case termbox.KeyHome, termbox.KeyCtrlA:
				E.cursorX = 0
			case termbox.KeyEnd, termbox.KeyCtrlE:
				if E.cursorY < E.numRows {
					E.cursorX = E.rows[E.cursorY].size
				}
			case termbox.KeyArrowDown, termbox.KeyArrowUp,
				termbox.KeyArrowLeft, termbox.KeyArrowRight:
				var key rune
				switch ev.Key {
				case termbox.KeyArrowDown:
					key = 'j'
				case termbox.KeyArrowUp:
					key = 'k'
				case termbox.KeyArrowLeft:
					key = 'h'
				case termbox.KeyArrowRight:
					key = 'l'
				}
				editorMoveCursor(key)
			case termbox.KeyPgdn, termbox.KeyPgup:
				// To scroll up or down a page, we position
				// the cursor either at the top or bottom of
				// the screen, and then simulate an entire
				// screen’s worth of ↑ or ↓ keypresses.
				var key rune
				if ev.Key == termbox.KeyPgdn {
					key = 'j'
					E.cursorY = E.rowOffset + E.screenRows - 1
					if E.cursorY > E.numRows {
						E.cursorY = E.numRows
					}
				} else {
					key = 'k'
					E.cursorY = E.rowOffset
				}

				times := E.screenRows
				for ; times > 0; times-- {
					editorMoveCursor(key)
				}
			default:
				// logger.Printf("ev: %+v\n", ev)
				if ev.Key == termbox.KeySpace || ev.Ch != 0 {
					keyPressed := ev.Ch
					if ev.Key == termbox.KeySpace {
						keyPressed = ' '
					}
					editorInsertChar(keyPressed)
				}
				// when pressing other keys, reset the quit time
				kiloQuitTimes = KILO_QUIT_TIMES
			}
		case termbox.EventError:
			panic(ev.Err)
		}

		editorRefreshScreen()
	}
}

func editorMoveCursor(ch rune) {
	switch ch {
	case 'h':
		if E.cursorX > 0 {
			E.cursorX--
		}
	case 'l':
		if E.cursorY < E.numRows {
			row := E.rows[E.cursorY]
			if E.cursorX < row.size {
				E.cursorX++
			}
		}
	case 'j':
		if E.cursorY < E.numRows {
			E.cursorY++
		}
	case 'k':
		if E.cursorY > 0 {
			E.cursorY--
		}
	}

	// 当移动到下一行的时候，移动到行末
	if E.cursorY != E.numRows {
		row := E.rows[E.cursorY]
		rowLen := row.size
		if E.cursorX > rowLen {
			if rowLen > 0 {
				E.cursorX = rowLen - 1
			} else {
				E.cursorX = 0
			}
		}
	} else {
		E.cursorX = 0
	}
}

func editorRefreshScreenSize() {
	w, h := termbox.Size()
	E.screenCols = w
	// save last two lines for status bar and status msg
	E.screenRows = h - 2
	E.statusBarRowIdx = h - 2
	E.msgBarRowIdx = h - 1
}

func initEditor() {
	E = &editorConf{}
	editorRefreshScreenSize()
}

func editorRefreshScreen() {
	editorScroll()
	termbox.SetCursor(E.renderCursorX-E.colOffset, E.cursorY-E.rowOffset)
	// get size again before redrawAll, because the
	// ui may be resized
	termbox.Clear(ColDef, ColDef)
	editorRefreshScreenSize()

	editorDrawRows()
	editorDrawStatusBar()
	editorDrawMsgbar()

	termbox.Flush()
}

func editorOpen(fileName string) {
	f, err := os.Open(fileName)
	if err != nil {
		panic(err)
	}
	reader := bufio.NewReader(f)
	var readErr error
	var line string
	line, readErr = reader.ReadString('\n')
	for readErr == nil {
		// remove newline
		end := len(line) - 1
		for end >= 0 && (line[end] == '\r' || line[end] == '\n') {
			end--
		}

		chars := line[:end+1]
		editorInsertRow(E.numRows, []rune(chars))

		line, readErr = reader.ReadString('\n')
	}
	E.filename = fileName
	E.modified = false
}

func editorSave() {
	if E.filename == "" {
		E.filename = editorPrompt("Save as: %v (ESC to cancle)")
		if E.filename == "" {
			editorSetStatusMsg("Save aborted")
			return
		}
	}
	var buffer bytes.Buffer
	for _, erow := range E.rows {
		buffer.WriteString(string(erow.rawChars))
		buffer.WriteRune('\n')
	}

	file, err := os.OpenFile(E.filename, os.O_RDWR|os.O_CREATE, 0644)
	if err == nil {
		// The normal way to overwrite a file is to pass the O_TRUNC
		// flag to open(), which truncates the file completely, making
		// it an empty file, before writing the new data into it.  By
		// truncating the file ourselves to the same length as the
		// data we are planning to write into it, we are making the
		// whole overwriting operation a little bit safer in case the
		// ftruncate() call succeeds but the write() call fails. In
		// that case, the file would still contain most of the data it
		// had before.
		if err = file.Truncate(int64(buffer.Len())); err == nil {
			if len, err := file.Write(buffer.Bytes()); err == nil {
				editorSetStatusMsg(fmt.Sprintf("%d bytes written to disk", len))
			}
		}
		file.Close()
		E.modified = false
		return
	}
	editorSetStatusMsg(fmt.Sprintf("Can't save! I/O error: %s", err.Error()))
}

func genRenderChars(rawChars []rune) []rune {
	var res []rune
	for _, ch := range rawChars {
		if ch == '\t' {
			for i := 0; i < KILO_TAB_STOP; i++ {
				res = append(res, ' ')
			}
		} else {
			res = append(res, ch)
		}
	}
	return res
}

func editorUpdateRow(erow *editorRow) {
	erow.renderChars = genRenderChars(erow.rawChars)
	erow.rsize = len(erow.renderChars)
}

func editorRowCxToRx(erow *editorRow, cx int) int {
	// rx := 0
	// for j := 0; j < cx; j++ {
	// 	if erow.rawChars[j] == '\t' {
	// 		rx += (KILO_TAB_STOP - 1) - (rx % KILO_TAB_STOP)
	// 	}
	// 	rx++
	// }
	// return rx

	// logger.Printf("erow: %+v, cx: %+v\n", erow, cx)
	tabs := 0
	widths := 0 // extra render space for non-ascii characters
	for j := 0; j < cx && j < len(erow.rawChars); j++ {
		width := runewidth.RuneWidth(erow.rawChars[j])
		if erow.rawChars[j] == '\t' {
			tabs++
		} else if width > 1 {
			widths += width - 1
		}
	}

	return cx + tabs*KILO_TAB_STOP - tabs + widths
}

func editorScroll() {
	E.renderCursorX = 0
	if E.cursorY < E.numRows {
		E.renderCursorX = editorRowCxToRx(E.rows[E.cursorY], E.cursorX)
	}

	if E.cursorY < E.rowOffset {
		E.rowOffset = E.cursorY
	}
	if E.cursorY >= E.rowOffset+E.screenRows {
		E.rowOffset = E.cursorY - E.screenRows + 1
	}

	if E.renderCursorX < E.colOffset {
		E.colOffset = E.renderCursorX
	}
	if E.renderCursorX >= E.colOffset+E.screenCols {
		E.colOffset = E.renderCursorX - E.screenCols + 1
	}
}

func editorDrawRows() {
	// fmt.Printf("cols: %v, rows: %v\n", EConf.screenCols, EConf.screenRows)
	for row := 0; row < E.screenRows; row++ {
		fileRow := row + E.rowOffset
		if fileRow >= E.numRows {
			if E.numRows == 0 && row == E.screenRows/3 {
				welcomeMsg := fmt.Sprintf("Kilo editor -- version %s", GKILO_VERSION)
				padding := (E.screenCols - len(welcomeMsg)) / 2

				termbox.SetCell(0, row, '~', ColWhi, ColDef)
				tbprint(padding, row, ColWhi,
					ColDef,
					welcomeMsg)
			}
			termbox.SetCell(0, row, '~', ColWhi, ColDef)
		} else {
			// TODO: 没搞懂
			// https://viewsourcecode.org/snaptoken/kilo/04.aTextViewer.html#horizontal-scrolling
			lineLen := E.rows[fileRow].size
			idx := lineLen - E.colOffset
			if idx < 0 {
				idx = 0
			}
			// logger.Printf("lineLen: %v, idx: %v\n", lineLen, idx)
			if idx > 0 {
				tbprint(0, row, ColWhi, ColDef, string(E.rows[fileRow].renderChars[E.colOffset:]))
			}
		}
	}
}

// editorInsertRow it supports insert after the last element
func editorInsertRow(rowIdx int, chars []rune) {
	if rowIdx < 0 || rowIdx > E.numRows {
		return
	}
	erow := editorRow{
		size: len(chars),
		// rawChars: []rune(chars),
		rawChars: make([]rune, 0, 512), // prealloc space to avoid resize frequently
	}
	erow.rawChars = erow.rawChars[:len(chars)] // make room for copy
	copy(erow.rawChars, chars)                 // dst cannot be empty when copying
	editorUpdateRow(&erow)

	// insert after the last row
	// just append it
	if rowIdx == E.numRows {
		E.rows = append(E.rows, &erow)
	} else {
		// insert the new row
		E.rows = append(E.rows, nil)
		copy(E.rows[rowIdx+1:], E.rows[rowIdx:])
		E.rows[rowIdx] = &erow
	}

	E.numRows++
	E.modified = true
}

// editorRowDelChar ...
func editorRowDelChar(erow *editorRow, at int) {
	if at < 0 || at >= erow.size {
		return
	}
	n := copy(erow.rawChars[at:], erow.rawChars[at+1:])
	logger.Printf("%d bytes copied", n)
	erow.size--
	erow.rawChars = erow.rawChars[:erow.size] // has to adjust slice size

	// // std lib now support slices.Delete
	// erow.rawChars = slices.Delete(erow.rawChars, at, at+1)

	editorUpdateRow(erow)
	E.modified = true
}

// editorDelChar ...
func editorDelChar() {
	if E.cursorY == E.numRows {
		return
	}

	erow := E.rows[E.cursorY]
	// if there is a character to the left of the cursor
	// we delete it and move the cursor one to the left
	if E.cursorX > 0 {
		editorRowDelChar(erow, E.cursorX-1)
		E.cursorX--
	} else {
		if E.cursorY > 0 {
			// append the remaining of the current row to the previous line
			prevRow := E.rows[E.cursorY-1]
			E.cursorX = prevRow.size
			editorRowAppendChars(prevRow, erow.rawChars...)
			// delete current row
			editorDelRow(E.cursorY)
			E.cursorY--
		}
	}
}

// editorDelRow delete the row at `rowIdx`
func editorDelRow(rowIdx int) {
	if rowIdx < 0 || rowIdx >= E.numRows {
		return
	}

	copy(E.rows[rowIdx:], E.rows[rowIdx+1:])
	E.numRows--
	E.rows = E.rows[:E.numRows]
	E.modified = true
}

// editorInsertNewline ...
func editorInsertNewline() {
	if E.cursorY < 0 || E.cursorY >= E.numRows {
		return
	}

	erow := E.rows[E.cursorY]
	if E.cursorX < 0 || E.cursorX > erow.size {
		return
	}
	// if we're at the beginning of a row, just insert a new empty row
	// before the current row
	if E.cursorX == 0 {
		editorInsertRow(E.cursorY, []rune(""))
	} else {
		var charsToMove []rune
		if E.cursorX != erow.size {
			charsToMove = erow.rawChars[E.cursorX:]
			erow.rawChars = erow.rawChars[:E.cursorX]
			erow.size -= len(charsToMove)
			editorUpdateRow(erow)
		}
		editorInsertRow(E.cursorY+1, charsToMove)
	}

	// update the cursor
	E.cursorX = 0
	E.cursorY++
}

func editorRowAppendChars(erow *editorRow, chars ...rune) {
	erow.rawChars = append(erow.rawChars, chars...)
	erow.size += len(chars)
	editorUpdateRow(erow)
}

func editorRowInsertChar(erow *editorRow, at int, c rune) {
	if at < 0 || at > erow.size {
		at = erow.size
	}
	// https://stackoverflow.com/a/46130603
	erow.rawChars = append(erow.rawChars, ' ')
	copy(erow.rawChars[at+1:], erow.rawChars[at:])
	erow.rawChars[at] = c

	// // std lib has slices.Insert since Go 1.21
	// erow.rawChars = slices.Insert(erow.rawChars, at, c)

	erow.size++
	editorUpdateRow(erow)
	logger.Printf("erow.size: %+v\n", erow.size)
	E.modified = true
}

func editorInsertChar(c rune) {
	if E.cursorY == E.numRows {
		// appendRow
		editorInsertRow(E.cursorY, []rune(""))
	}
	erow := E.rows[E.cursorY]
	editorRowInsertChar(erow, E.cursorX, c)
	logger.Printf("rawChars: %c\n", erow.rawChars)
	// logger.Printf("renderChars: %c\n", erow.renderChars)
	E.cursorX++
}

func editorDrawStatusBar() {
	fgColor := termbox.ColorBlack
	bgColor := termbox.ColorWhite
	filename := "[No Name]"
	if E.filename != "" {
		filename = E.filename
	}
	dirtyMsg := ""
	if E.modified {
		dirtyMsg = "(modified)"
	}
	msg := fmt.Sprintf("%.*s - %d lines %s", FILENAME_MAX_PRINT, filename, E.numRows, dirtyMsg)
	tbprint(0, E.statusBarRowIdx, fgColor, bgColor, msg)
	for i := 0; i < E.screenCols-len(msg); i++ {
		termbox.SetCell(len(msg)+i, E.statusBarRowIdx, rune(' '), fgColor, bgColor)
	}
}

// editorDrawMsgbar ...
func editorDrawMsgbar() {
	now := time.Now()
	if now.Sub(E.statusMsgTime) < 5*time.Second {
		// 使用 "%.*s" 格式说明符，其中 * 表示动态指定宽度。
		msg := fmt.Sprintf("%.*s", E.screenCols, E.statusMsg)
		tbprint(0, E.msgBarRowIdx, ColWhi, ColDef, msg)
	}
}

// editorSetStatusMsg ...
func editorSetStatusMsg(strFmt string, args ...any) {
	var msg string
	if len(args) > 0 {
		msg = fmt.Sprintf(strFmt, args...)
	} else {
		msg = strFmt
	}
	E.statusMsg = msg
	E.statusMsgTime = time.Now()
}

func editorPrompt(prompt string) string {
	var buffer bytes.Buffer

	for {
		editorSetStatusMsg(prompt, buffer.String())
		editorRefreshScreen()

		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			if ev.Ch != 0 {
				buffer.WriteRune(ev.Ch)
			} else if ev.Key == termbox.KeyEnter {
				editorSetStatusMsg("")
				return buffer.String()
			} else if ev.Key == termbox.KeyEsc {
				editorSetStatusMsg("")
				return ""
			} else if ev.Key == termbox.KeyBackspace2 || ev.Key == termbox.KeyDelete {
				if buffer.Len() > 0 {
					buffer.Truncate(buffer.Len() - 1)
				}
			}
		}
	}
}

// This function is often use
func tbprint(x, y int, fg, bg termbox.Attribute, msg string) {
	for _, c := range msg {
		termbox.SetCell(x, y, c, fg, bg)
		x += runewidth.RuneWidth(c)
	}
}
