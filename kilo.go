package main

import (
	"bufio"
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
	rows            []editorRow
	numRows         int
	rowOffset       int
	colOffset       int
	filename        string
	statusMsg       string
	statusMsgTime   time.Time // the timestamp when we set a statusMsg
}

var E *editorConf
var logger *log.Logger

func main() {
	fileNamePtr := flag.String("f", "", "file to open")

	flag.Parse()

	logfile, err := os.OpenFile(LogFile, os.O_CREATE|os.O_WRONLY, 0644)
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

	editorSetStatusMsg("HELP: C-Q = quit")

	editorRefreshScreen()

loop:
	for {
		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			switch ev.Key {
			case termbox.KeyCtrlQ:
				break loop
			case termbox.KeyEsc:
				break loop
			case termbox.KeyHome:
				E.cursorX = 0
			case termbox.KeyEnd:
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
				if ev.Ch != 0 {
					// // print
					// putChar(x, y, fgColor, bgColor, ev.Ch)
					// x += runewidth.RuneWidth(ev.Ch)
					editorMoveCursor(ev.Ch)
				}
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
		row := E.rows[E.cursorY]
		if E.cursorX < row.size-1 {
			E.cursorX++
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

		data := line[:end+1]
		// logger.Printf("lenData: %v, lenLine: %v\n", len(data), len(line))
		renderChars := genRenderChars([]rune(data))

		E.rows = append(E.rows,
			editorRow{
				size:        len(data),
				rawChars:    []rune(data),
				renderChars: renderChars,
				rsize:       len(renderChars),
			})
		E.numRows++

		line, readErr = reader.ReadString('\n')
	}
	E.filename = fileName
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

func editorRowCxToRx(erow *editorRow, cx int) int {
	// rx := 0
	// for j := 0; j < cx; j++ {
	// 	if erow.rawChars[j] == '\t' {
	// 		rx += (KILO_TAB_STOP - 1) - (rx % KILO_TAB_STOP)
	// 	}
	// 	rx++
	// }
	// return rx

	tabs := 0
	for j := 0; j < cx; j++ {
		if erow.rawChars[j] == '\t' {
			tabs++
		}
	}

	return cx + tabs*KILO_TAB_STOP - tabs
}

func editorScroll() {
	E.renderCursorX = 0
	if E.cursorY < E.numRows {
		E.renderCursorX = editorRowCxToRx(&E.rows[E.cursorY], E.cursorX)
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
			if idx > 0 {
				tbprint(0, row, ColWhi, ColDef, string(E.rows[fileRow].renderChars[E.colOffset:]))
			}
		}
	}
}

func editorDrawStatusBar() {
	fgColor := termbox.ColorBlack
	bgColor := termbox.ColorWhite
	filename := "[No Name]"
	if E.filename != "" {
		filename = E.filename
	}
	msg := fmt.Sprintf("%.*s - %d lines", FILENAME_MAX_PRINT, filename, E.numRows)
	tbprint(0, E.statusBarRowIdx, fgColor, bgColor, msg)
	for i := 0; i < E.screenCols-len(msg); i++ {
		termbox.SetCell(len(msg)+i, E.statusBarRowIdx, rune(' '), fgColor, bgColor)
	}
}

// editorDrawMsgbar ...
func editorDrawMsgbar() {
	// 使用 "%.*s" 格式说明符，其中 * 表示动态指定宽度。
	msg := fmt.Sprintf("%.*s", E.screenCols, E.statusMsg)
	tbprint(0, E.msgBarRowIdx, ColWhi, ColDef, msg)
}

// editorSetStatusMsg ...
func editorSetStatusMsg(msg string) {
	E.statusMsg = msg
	E.statusMsgTime = time.Now()
}

// This function is often use
func tbprint(x, y int, fg, bg termbox.Attribute, msg string) {
	for _, c := range msg {
		termbox.SetCell(x, y, c, fg, bg)
		x += runewidth.RuneWidth(c)
	}
}
