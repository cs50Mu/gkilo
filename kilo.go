package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/mattn/go-runewidth"
	tb "github.com/nsf/termbox-go"
)

const (
	GKILO_VERSION      = "0.0.1"
	ColDef             = tb.ColorDefault
	ColWhi             = tb.ColorWhite
	LogFile            = "kilo.log"
	KILO_TAB_STOP      = 4
	FILENAME_MAX_PRINT = 20
	KILO_QUIT_TIMES    = 3
)

type editorHighlight uint8

const (
	HL_NORMAl editorHighlight = iota
	HL_NUMBER
	HL_STRING
	HL_COMMENT
	HL_MLCOMMENT
	HL_KEYWORD1
	HL_KEYWORD2
	HL_MATCH
)

type editorRow struct {
	idx           int
	size          int
	rawChars      []rune // 原始的字符集合
	rsize         int
	renderChars   []rune            // 需要渲染的字符集合
	hl            []editorHighlight // for highlighting
	hlOpenComment bool
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
	syntax          *editorSyntax
}

type editorSyntax struct {
	fileType  string
	fileMatch []string // an array of strings, where each string
	// contains a pattern to match a filename against
	singlelineCommentStart string // start of single line comment, eg,
	// `//` in golang or c
	multilineCommentStart string
	multilineCommentEnd   string
	keywords              []string
	flags                 int
}

const (
	HL_HIGHLIGHT_NUMBERS = 1 << 0
	HL_HIGHLIGHT_STRINGS = 1 << 1
)

/***** filetypes *****/
var (
	C_HL_EXTENSIONS = []string{".c", ".h", ".cpp"}
	C_HL_KEYWORDS   = []string{
		"switch", "if", "while", "for", "break", "continue", "return", "else",
		"struct", "union", "typedef", "static", "enum", "class", "case",
		"int|", "long|", "double|", "float|", "char|", "unsigned|", "signed|",
		"void|",
	}

	HLDB = []editorSyntax{
		{
			fileType:               "c",
			fileMatch:              C_HL_EXTENSIONS,
			singlelineCommentStart: "//",
			multilineCommentStart:  "/*",
			multilineCommentEnd:    "*/",
			keywords:               C_HL_KEYWORDS,
			flags:                  HL_HIGHLIGHT_NUMBERS | HL_HIGHLIGHT_STRINGS,
		},
	}
)

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

	err = tb.Init()
	if err != nil {
		panic(err)
	}
	defer tb.Close()

	tb.SetInputMode(tb.InputEsc)

	initEditor()

	if *fileNamePtr != "" {
		editorOpen(*fileNamePtr)
	}

	editorSetStatusMsg("HELP: C-X C-S = save | C-X C-C = quit | C-S = find")
	editorRefreshScreen()
	editorProcessKeypress()
}

// editorProcessKeypress ...
func editorProcessKeypress() {
	kiloQuitTimes := KILO_QUIT_TIMES
	var prevKey tb.Key
loop:
	for {
		switch ev := tb.PollEvent(); ev.Type {
		case tb.EventKey:
			switch ev.Key {
			case tb.KeyCtrlC:
				if prevKey == tb.KeyCtrlX {
					kiloQuitTimes--
					if E.modified && kiloQuitTimes > 0 {
						editorSetStatusMsg(fmt.Sprintf("WARNING!!! File has unsaved changes. Press C-X C-C %d more times to quit.", kiloQuitTimes))
					} else {
						break loop
					}
				}
			case tb.KeyEsc:
				break loop
			case tb.KeyEnter:
				editorInsertNewline()
			// Backspace delete the character to the left of the cursor
			// Del delete the character under the cursor
			case tb.KeyBackspace2, tb.KeyDelete:
				if ev.Key == tb.KeyDelete {
					editorMoveCursor(tb.KeyArrowRight)
				}
				editorDelChar()
			// case tb.KeyDelete:
			case tb.KeyCtrlL:
				editorDelRow(E.cursorY)
			case tb.KeyCtrlS:
				if prevKey == tb.KeyCtrlX {
					editorSave()
				} else {
					editorFind()
				}
			case tb.KeyCtrlX:
				prevKey = ev.Key
			case tb.KeyHome, tb.KeyCtrlA:
				E.cursorX = 0
			case tb.KeyEnd, tb.KeyCtrlE:
				if E.cursorY < E.numRows {
					E.cursorX = E.rows[E.cursorY].size
				}
			case tb.KeyArrowDown, tb.KeyArrowUp,
				tb.KeyArrowLeft, tb.KeyArrowRight,
				tb.KeyCtrlN, tb.KeyCtrlP, tb.KeyCtrlB, tb.KeyCtrlF:
				editorMoveCursor(ev.Key)
			case tb.KeyPgdn, tb.KeyPgup:
				// To scroll up or down a page, we position
				// the cursor either at the top or bottom of
				// the screen, and then simulate an entire
				// screen’s worth of ↑ or ↓ keypresses.
				var key tb.Key
				if ev.Key == tb.KeyPgdn {
					key = tb.KeyArrowDown
					E.cursorY = E.rowOffset + E.screenRows - 1
					if E.cursorY > E.numRows {
						E.cursorY = E.numRows
					}
				} else {
					key = tb.KeyArrowUp
					E.cursorY = E.rowOffset
				}

				times := E.screenRows
				for ; times > 0; times-- {
					editorMoveCursor(key)
				}
			default:
				// logger.Printf("ev: %+v\n", ev)
				if ev.Key == tb.KeySpace || ev.Ch != 0 {
					keyPressed := ev.Ch
					if ev.Key == tb.KeySpace {
						keyPressed = ' '
					}
					editorInsertChar(keyPressed)
				}
				// when pressing other keys, reset the quit time
				kiloQuitTimes = KILO_QUIT_TIMES
			}
		case tb.EventError:
			panic(ev.Err)
		}

		editorRefreshScreen()
	}
}

func editorMoveCursor(key tb.Key) {
	switch key {
	case tb.KeyArrowDown, tb.KeyCtrlN:
		if E.cursorY < E.numRows {
			E.cursorY++
		}
	case tb.KeyArrowUp, tb.KeyCtrlP:
		if E.cursorY > 0 {
			E.cursorY--
		}
	case tb.KeyArrowLeft, tb.KeyCtrlB:
		if E.cursorX > 0 {
			E.cursorX--
		}
	case tb.KeyArrowRight, tb.KeyCtrlF:
		if E.cursorY < E.numRows {
			row := E.rows[E.cursorY]
			if E.cursorX < row.size {
				E.cursorX++
			}
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
	w, h := tb.Size()
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
	tb.SetCursor(E.renderCursorX-E.colOffset, E.cursorY-E.rowOffset)
	// get size again before redrawAll, because the
	// ui may be resized
	tb.Clear(ColDef, ColDef)
	editorRefreshScreenSize()

	editorDrawRows()
	editorDrawStatusBar()
	editorDrawMsgbar()

	tb.Flush()
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
	editorSelectSyntaxHighlight()
}

func editorSave() {
	if E.filename == "" {
		E.filename = editorPrompt("Save as: %v (ESC to cancle)", nil)
		if E.filename == "" {
			editorSetStatusMsg("Save aborted")
			return
		}
		editorSelectSyntaxHighlight()
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
	editorUpdateSyntax(erow)
}

// editorRowCxToRx CursorX --> renderCursorX
func editorRowCxToRx(erow *editorRow, cx int) int {
	var rx int
	for i := 0; i < cx; i++ {
		width := runewidth.RuneWidth(erow.rawChars[i])
		if erow.rawChars[i] == '\t' {
			rx += KILO_TAB_STOP
		} else {
			rx += width
		}
	}

	return rx
}

// editorRowRxToCx renderCursorX --> cursorX
func editorRowRxToCx(erow *editorRow, rx int) int {
	currRx := 0
	var i int
	for i = 0; i < erow.size; i++ {
		width := runewidth.RuneWidth(erow.rawChars[i])
		if erow.rawChars[i] == '\t' {
			currRx += KILO_TAB_STOP
		} else {
			currRx += width
		}
		if currRx > rx {
			break
		}
	}
	return i
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

				tb.SetCell(0, row, '~', ColWhi, ColDef)
				tbprint(padding, row, ColWhi,
					ColDef,
					welcomeMsg)
			}
			tb.SetCell(0, row, '~', ColWhi, ColDef)
		} else {
			// https://viewsourcecode.org/snaptoken/kilo/04.aTextViewer.html#horizontal-scrolling
			lineLen := E.rows[fileRow].size
			displayLen := lineLen - E.colOffset
			if displayLen < 0 {
				displayLen = 0
			}
			// logger.Printf("lineLen: %v, idx: %v\n", lineLen, idx)
			if displayLen > 0 {
				erow := E.rows[fileRow]
				chars := erow.renderChars[E.colOffset:] // display from offset
				colIdx := 0
				if displayLen > E.screenCols {
					displayLen = E.screenCols // there are at most E.screenCols space to display
				}
				for i := 0; colIdx < displayLen; i++ {
					if unicode.IsControl(chars[i]) {
						var sym rune
						if chars[i] <= 26 {
							sym = '@' + chars[i]
						} else {
							sym = '?'
						}
						// use inverted color
						tb.SetCell(colIdx, row, sym, ColDef, ColWhi)
					} else {
						textColor := editorSyntaxToColor(erow.hl[i])
						tb.SetCell(colIdx, row, chars[i], textColor, ColDef)
					}
					colIdx += runewidth.RuneWidth(erow.renderChars[i])
				}
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
		idx:      rowIdx,
		size:     len(chars),
		rawChars: chars,
	}
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
		// update rowIdx
		for i := rowIdx + 1; i <= E.numRows; i++ {
			E.rows[i].idx += 1
		}
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
	for i := rowIdx; i < E.numRows; i++ {
		E.rows[i].idx -= 1
	}
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
	// logger.Printf("erow.size: %+v\n", erow.size)
	E.modified = true
}

func editorInsertChar(c rune) {
	if E.cursorY == E.numRows {
		// appendRow
		editorInsertRow(E.cursorY, []rune(""))
	}
	erow := E.rows[E.cursorY]
	editorRowInsertChar(erow, E.cursorX, c)
	// logger.Printf("rawChars: %c\n", erow.rawChars)
	// logger.Printf("renderChars: %c\n", erow.renderChars)
	E.cursorX++
}

func editorDrawStatusBar() {
	fgColor := tb.ColorBlack
	bgColor := tb.ColorWhite
	filename := "[No Name]"
	if E.filename != "" {
		filename = E.filename
	}
	dirtyMsg := ""
	if E.modified {
		dirtyMsg = "(modified)"
	}
	// msg at the left end of the status bar
	lMsg := fmt.Sprintf("%.*s - %d lines %s", FILENAME_MAX_PRINT, filename, E.numRows, dirtyMsg)
	// msg at the right end of the status bar
	fileTypeDisp := "no ft"
	if E.syntax != nil {
		fileTypeDisp = E.syntax.fileType
	}
	rMsg := fmt.Sprintf("%s | %d/%d", fileTypeDisp, E.cursorY+1, E.numRows)
	printLen := len(lMsg)
	// print at most `E.screenCols` chars
	if printLen > E.screenCols {
		printLen = E.screenCols
	}
	tbprint(0, E.statusBarRowIdx, fgColor, bgColor, lMsg[:printLen])
	for printLen < E.screenCols {
		if E.screenCols-printLen == len(rMsg) {
			tbprint(printLen, E.statusBarRowIdx, fgColor, bgColor, rMsg)
			break
		}
		tb.SetCell(printLen, E.statusBarRowIdx, ' ', fgColor, bgColor)
		printLen++
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

func editorPrompt(prompt string,
	cb func(query string, lastKey tb.Key)) string {
	var buffer bytes.Buffer

	for {
		editorSetStatusMsg(prompt, buffer.String())
		editorRefreshScreen()

		switch ev := tb.PollEvent(); ev.Type {
		case tb.EventKey:
			if ev.Ch != 0 {
				buffer.WriteRune(ev.Ch)
			} else if ev.Key == tb.KeyEnter {
				editorSetStatusMsg("")
				// cb need to be called here to let
				// `editorFindCallback` get a chance to know about the
				// event
				cb(buffer.String(), ev.Key)
				return buffer.String()
			} else if ev.Key == tb.KeyEsc {
				editorSetStatusMsg("")
				cb(buffer.String(), ev.Key)
				return ""
			} else if ev.Key == tb.KeyBackspace2 || ev.Key == tb.KeyDelete {
				if buffer.Len() > 0 {
					buffer.Truncate(buffer.Len() - 1)
				}
			}

			if cb != nil {
				cb(buffer.String(), ev.Key)
			}
		}
	}
}

func editorFind() {
	savedCx := E.cursorX
	savedCy := E.cursorY
	savedRowOffset := E.rowOffset
	savedColOffset := E.colOffset
	query := editorPrompt("Search: %s (Use ESC/Arrow/Enter)", editorFindCallback)
	if query == "" {
		E.cursorX = savedCx
		E.cursorY = savedCy
		E.rowOffset = savedRowOffset
		E.colOffset = savedColOffset
	}
}

var (
	lastMatch   int = -1
	direction   int = 1
	savedHL     []editorHighlight
	savedHLline int
)

func editorFindCallback(query string, lastKey tb.Key) {
	if savedHL != nil {
		copy(E.rows[savedHLline].hl, savedHL)
		savedHL = nil
	}
	// when in `incremental search`, press Enter or Esc means the search is done
	if lastKey == tb.KeyEnter || lastKey == tb.KeyEsc {
		lastMatch = -1
		direction = 1
		return
	} else if lastKey == tb.KeyArrowDown {
		direction = 1
	} else if lastKey == tb.KeyArrowUp {
		direction = -1
	} else { // 当不是方向键时，还是从头开始搜索
		lastMatch = -1
		direction = 1
	}
	if lastMatch == -1 { // if there is no lastMatch, search in the
		// forward direction
		direction = 1
	}
	curr := lastMatch
	// TODO: current implemention continues to search in the "next" line, but
	// the next match may still exists in the current line
	for i := 0; i < E.numRows; i++ { // 最多搜索 numRows 行，思考一下 wrap around
		curr += direction // 根据 direction 来移动到“下一行”来继续搜索
		if curr == -1 {
			curr = E.numRows - 1
		} else if curr == E.numRows {
			curr = 0
		} // allow search to wrap around
		erow := E.rows[curr]
		rx := strings.Index(string(erow.renderChars), query)
		// logger.Printf("query: %v, rx: %v\n", query, rx)
		if rx >= 0 {
			E.cursorY = curr
			lastMatch = curr
			// cursorX need a cx
			E.cursorX = editorRowRxToCx(erow, rx)
			// we set E.rowOffset so that we are scrolled to the very
			// bottom of the file, which will cause editorScroll() to
			// scroll upwards at the next screen refresh so that the
			// matching line will be at the very top of the
			// screen. This way, the user doesn’t have to look all
			// over their screen to find where their cursor jumped to,
			// and where the matching line is.
			E.rowOffset = E.numRows
			// save original hl for restore first
			savedHLline = curr
			savedHL = make([]editorHighlight, erow.rsize)
			copy(savedHL, erow.hl)
			// highlight the query
			for i := rx; i < rx+len(query); i++ {
				erow.hl[i] = HL_MATCH
			}
			break
		}
	}
}

// This function is often use
func tbprint(x, y int, fg, bg tb.Attribute, msg string) {
	for _, c := range msg {
		tb.SetCell(x, y, c, fg, bg)
		x += runewidth.RuneWidth(c)
	}
}

const (
	SEPS = ",.()+-/*=~%<>[];，；。：（）"
)

/***** syntax highlighting *****/
func isSeparator(c rune) bool {
	return unicode.IsSpace(c) || strings.ContainsRune(SEPS, c)
}

func editorUpdateSyntax(erow *editorRow) {
	if erow.hl == nil {
		erow.hl = make([]editorHighlight, erow.rsize)
	}
	// make erow.hl as big as rsize
	if erow.rsize < len(erow.hl) {
		erow.hl = erow.hl[:erow.rsize]
	} else if erow.rsize > len(erow.hl) {
		erow.hl = append(erow.hl, make([]editorHighlight, erow.rsize-len(erow.hl))...)
	}
	// memset to default color first
	for i := 0; i < len(erow.hl); i++ {
		erow.hl[i] = HL_NORMAl
	}
	if E.syntax == nil {
		return
	}

	keywords := E.syntax.keywords
	scs := E.syntax.singlelineCommentStart
	mcs := E.syntax.multilineCommentStart
	mce := E.syntax.multilineCommentEnd
	var i int
	preSep := true
	// used to indicate whether in a string currently, also used to
	// store the quotes (" or ')
	inStr := rune(0)
	inComment := erow.idx > 0 && E.rows[erow.idx-1].hlOpenComment
	for i = 0; i < erow.rsize; {
		var prevHL editorHighlight
		if i > 0 {
			prevHL = erow.hl[i-1]
		} else {
			prevHL = HL_NORMAl
		}

		// inStr == rune(0) means not in a string
		// comment
		if scs != "" && inStr == rune(0) && !inComment {
			if strings.HasPrefix(string(erow.renderChars[i:]), scs) {
				for j := i; j < erow.rsize; j++ { // make hl[i..] HL_COMMENT
					erow.hl[j] = HL_COMMENT
				}
				break
			}
		}
		if mcs != "" && mce != "" && inStr == rune(0) { // not in str
			if inComment {
				erow.hl[i] = HL_MLCOMMENT
				if strings.HasPrefix(string(erow.renderChars[i:]), mce) { // we met the end of ml_comment
					for j := i; j < i+len(mce); j++ {
						erow.hl[j] = HL_MLCOMMENT
					}
					i += len(mce)
					preSep = true
					inComment = false
					continue
				} else {
					i++
					continue
				}
			} else if strings.HasPrefix(string(erow.renderChars[i:]), mcs) { // we met the opening of ml_comment
				// logger.Printf("renderChars: %+v\n", string(erow.renderChars[i:]))
				for j := i; j < i+len(mcs); j++ {
					erow.hl[j] = HL_MLCOMMENT
				}
				i += len(mcs)
				inComment = true
				continue
			}
		}
		char := erow.renderChars[i]
		// string
		if E.syntax.flags&HL_HIGHLIGHT_STRINGS != 0 {
			if inStr != rune(0) { // in a string
				erow.hl[i] = HL_STRING
				// take care of escaped quotes
				if char == '\\' && i+1 < erow.rsize { // it's a `\`
					erow.hl[i+1] = HL_STRING
					i += 2
					continue
				}
				if char == inStr {
					inStr = rune(0) // met the ending of string, reset `inStr`
				}
				i++
				preSep = true
				continue
			} else { // not in a string
				if char == '"' || char == '\'' {
					erow.hl[i] = HL_STRING
					inStr = char
					i++
					continue
				}
			}
		}
		// number
		if E.syntax.flags&HL_HIGHLIGHT_NUMBERS != 0 {
			if (unicode.IsDigit(char) && (preSep || prevHL == HL_NUMBER)) || (char == '.' && prevHL == HL_NUMBER) {
				erow.hl[i] = HL_NUMBER
				i++
				preSep = false
				continue
			}
		}
		// keywords
		if preSep {
			var foundKeyword bool
			for _, keyword := range keywords {
				var isKw2 bool
				if strings.HasSuffix(keyword, "|") {
					keyword = keyword[:len(keyword)-1] // remove trailing pipe `|`
					isKw2 = true
				}
				// pre is sep && keyword match && (we met lineEnd || next char is sep also)
				if strings.HasPrefix(string(erow.renderChars[i:]), keyword) &&
					(i+len(keyword) >= erow.rsize ||
						isSeparator(erow.renderChars[i+len(keyword)])) {
					for j := i; j < i+len(keyword); j++ {
						if isKw2 {
							erow.hl[j] = HL_KEYWORD2
						} else {
							erow.hl[j] = HL_KEYWORD1
						}
					}
					i += len(keyword)
					foundKeyword = true
					break
				}
			}
			if foundKeyword {
				preSep = false
				continue
			}
		}
		preSep = isSeparator(char)
		i++
	}
	changed := erow.hlOpenComment != inComment
	erow.hlOpenComment = inComment
	if changed && erow.idx+1 < E.numRows {
		editorUpdateSyntax(E.rows[erow.idx+1])
	}
}

func editorSyntaxToColor(hl editorHighlight) tb.Attribute {
	switch hl {
	case HL_NUMBER:
		return tb.ColorRed
	case HL_MATCH:
		return tb.ColorLightBlue
	case HL_STRING:
		return tb.ColorMagenta
	case HL_COMMENT, HL_MLCOMMENT:
		return tb.ColorCyan
	case HL_KEYWORD1:
		return tb.ColorYellow
	case HL_KEYWORD2:
		return tb.ColorGreen
	default:
		return tb.ColorDefault
	}
}

func editorSelectSyntaxHighlight() {
	E.syntax = nil
	if E.filename == "" {
		return
	}
	parts := strings.Split(E.filename, ".")
	if len(parts) != 2 {
		logger.Printf("[WARN] not a proper filename: %v, disable syntax highlighting", E.filename)
		return
	}
	fileExt := parts[1]
	for _, hl := range HLDB {
		for _, m := range hl.fileMatch {
			isExt := strings.HasPrefix(m, ".")
			if (isExt && fileExt == m[1:]) || (!isExt && strings.Contains(E.filename, m)) {
				E.syntax = &hl
				// update the syntax of every row
				for i := 0; i < E.numRows; i++ {
					editorUpdateSyntax(E.rows[i])
				}
				return
			}
		}
	}
}
