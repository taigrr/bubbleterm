package emulator

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
)

type dupReader struct {
	reader *bufio.Reader
	buf    *bytes.Buffer
	e      *Emulator
}

func (r *dupReader) ReadByte() (byte, error) {
	b, err := r.reader.ReadByte()
	if r.buf != nil {
		r.buf.Write([]byte{b})
	}
	return b, err
}

func (r *dupReader) ReadRune() (rune, int, error) {
	b, n, err := r.reader.ReadRune()
	if err == nil {
		byts := []byte(string(b))
		if r.buf != nil {
			r.buf.Write(byts)
		}
	}
	return b, n, err
}

func (r *dupReader) Buffered() int {
	return r.reader.Buffered()
}

func (e *Emulator) ptyReadLoop() {
	r := &dupReader{
		reader: bufio.NewReader(e.pty),
		buf:    nil,
		e:      e,
	}

	for {
		select {
		case <-e.stopChan:
			return
		default:
		}

		b, _, err := r.ReadRune()
		if err != nil {
			return
		}

		// printables
		runes := []rune{}
		for b >= 32 && (b <= 126 || b >= 128) {

			runes = append(runes, rune(b))

			if r.Buffered() == 0 {
				break
			}

			b, _, err = r.ReadRune()
			if err != nil {
				return
			}
		}
		if len(runes) > 0 {
			e.mu.Lock()
			e.currentScreen().writeRunes(runes)
			e.mu.Unlock()

			if r.Buffered() == 0 {
				continue
			}
		}

		switch b {
		case 0: // NUL Null byte, ignore

		case 7: // BEL ^G Bell
			// Bell - could emit event here

		case 8: // BS ^H Backspace
			e.mu.Lock()
			e.currentScreen().moveCursor(-1, 0, false, false)
			e.mu.Unlock()

		case 9: // HT ^I Horizontal TAB
			// TODO: tab

		case 10: // LF ^J Linefeed (newline)
			e.mu.Lock()
			e.currentScreen().moveCursor(0, 1, true, true)
			e.mu.Unlock()

		case 11: // VT ^K Vertical TAB
			// TODO: vtab

		case 12: // FF ^L Formfeed (also: New page NP)
			e.mu.Lock()
			e.currentScreen().moveCursor(0, 1, false, true)
			e.mu.Unlock()

		case 13: // CR ^M Carriage Return
			e.mu.Lock()
			e.currentScreen().moveCursor(-e.currentScreen().cursorPos.X, 0, true, true)
			e.mu.Unlock()

		case 27: // ESC ^[ Escape Character
			var cmdBytes bytes.Buffer
			r.buf = &cmdBytes

			e.mu.Lock()
			success := e.handleCommand(r)
			r.buf = nil
			e.mu.Unlock()

			if !success {
				// If we didn't handle the command, we can just ignore it for now
				_ = true // ignore unused success check for now
			}
			continue

		case 127: // DEL  Delete Character
			e.mu.Lock()
			e.currentScreen().eraseRegion(Region{
				X:  e.currentScreen().cursorPos.X,
				Y:  e.currentScreen().cursorPos.Y,
				X2: e.currentScreen().cursorPos.X + 1,
				Y2: e.currentScreen().cursorPos.Y + 1,
			}, CRClear)
			e.mu.Unlock()

		default:
			// unhandled char, undefined behavior
			continue
		}
	}
}

func (e *Emulator) handleCommand(r *dupReader) bool {
	b, _, err := r.ReadRune()
	if err != nil {
		return false
	}

	// short commands
	switch b {
	case 'c': // reset
		// TODO: reset

	case 'D': // Index, scroll down if necessary
		e.currentScreen().moveCursor(0, 1, false, true)

	case 'M': // Reverse index, scroll up if necessary
		e.currentScreen().moveCursor(0, -1, false, true)

	case '[': // CSI Control Sequence Introducer
		return e.handleCmdCSI(r)

	case ']': // OSC Operating System Commands
		return e.handleCmdOSC(r)

	case '(': // G0
		fallthrough
	case ')': // G1
		fallthrough
	case '*': // G2
		fallthrough
	case '+': // G3
		_, _, err := r.ReadRune()
		if err != nil {
			return false
		}
		// TODO: Character Set

	case '=': // Application Keypad
		// TODO: Application Keypad

	case '>': // Normal Keypad
		// TODO: Normal Keypad

	default:
		return false
	}
	return true
}

func (e *Emulator) handleCmdCSI(r *dupReader) bool {
	b, _, err := r.ReadRune()
	if err != nil {
		return false
	}

	prefix := []byte{}
	if b == '?' || b == '>' {
		prefix = append(prefix, byte(b))

		b, _, err = r.ReadRune()
		if err != nil {
			return false
		}
	}

	paramBytes := []byte{}
	for b == ';' || (b >= '0' && b <= '9') {
		paramBytes = append(paramBytes, byte(b))

		b, _, err = r.ReadRune()
		if err != nil {
			return false
		}
	}

	paramParts := strings.Split(string(paramBytes), ";")
	if len(paramParts) == 1 && paramParts[0] == "" {
		paramParts = []string{}
	}
	params := make([]int, len(paramParts))
	for i, p := range paramParts {
		params[i], _ = strconv.Atoi(p)
	}

	if string(prefix) == "" {
		switch b {
		case 'A': // Move cursor up
			if len(params) == 0 {
				params = []int{1}
			}
			e.currentScreen().moveCursor(0, -params[0], false, true)
		case 'B': // Move cursor down
			if len(params) == 0 {
				params = []int{1}
			}
			e.currentScreen().moveCursor(0, params[0], false, true)
		case 'C': // Move cursor forward
			if len(params) == 0 {
				params = []int{1}
			}
			e.currentScreen().moveCursor(params[0], 0, false, false)
		case 'D': // Move cursor backward
			if len(params) == 0 {
				params = []int{1}
			}
			e.currentScreen().moveCursor(-params[0], 0, false, false)

		case 'G': // Cursor Character Absolute
			if len(params) == 0 {
				params = []int{1}
			}
			e.currentScreen().setCursorPos(params[0]-1, e.currentScreen().cursorPos.Y)

		case 'c': // Send Device Attributes
			if len(params) == 0 {
				params = []int{1}
			}
			switch params[0] {
			case 0:
				e.pty.Write([]byte("\033[?1;2c"))
			}

		case 'd': // Line Position Absolute
			if len(params) == 0 {
				params = []int{1}
			}
			e.currentScreen().setCursorPos(e.currentScreen().cursorPos.X, params[0]-1)

		case 'f', 'H': // Cursor Home
			x := 1
			y := 1
			if len(params) >= 1 {
				y = params[0]
			}
			if len(params) >= 2 {
				x = params[1]
			}
			e.currentScreen().setCursorPos(x-1, y-1)

		case 'h', 'l': // h=Set, l=Reset Mode
			// set := b == 'h'

			if len(params) != 1 {
				return false
			}

			switch params[0] {
			case 4:
				// TODO: Insert Mode
			default:
				return false
			}

		case 'm': // Set color/mode
			if len(params) == 0 {
				params = []int{0}
			}

			screen := e.currentScreen()
			fc := screen.frontColor
			bc := screen.backColor

			for i := 0; i < len(params); i++ {
				p := params[i]
				switch {
				case p == 0: // reset mode
					fc = ColWhite
					bc = ColBlack

				case p >= 1 && p <= 8:
					fc = fc.SetMode(ColorModes[p-1])

				case p == 22:
					fc = fc.ResetMode(ModeBold).ResetMode(ModeDim)

				case p == 23:
					fc = fc.ResetMode(ModeItalic)

				case p == 24:
					fc = fc.ResetMode(ModeUnderline)

				case p == 27:
					fc = fc.ResetMode(ModeReverse)

				case p >= 30 && p <= 37:
					fc = fc.SetColor(Colors8[p-30])

				case p == 39: // default color
					fc = fc.SetColor(ColWhite)

				case p >= 40 && p <= 47:
					bc = bc.SetColor(Colors8[p-40])

				case p == 49: // default color
					bc = bc.SetColor(ColBlack)

				case p == 38 || p == 48: // extended set color
					if i+2 < len(params) {
						switch params[i+1] {
						case 5: // 256 color
							if p == 38 {
								fc = fc.SetColor(Color(params[i+2] & 0xff))
							} else {
								bc = bc.SetColor(Color(params[i+2] & 0xff))
							}
							i += 2
						case 2: // RGB Color
							if i+4 < len(params) {
								if p == 38 {
									fc = fc.SetColorRGB(params[i+2], params[i+3], params[i+4])
								} else {
									bc = bc.SetColorRGB(params[i+2], params[i+3], params[i+4])
								}
								i += 4
							}
						default:
							continue
						}
					}

				case p >= 90 && p <= 97:
					fc = fc.SetColor(Color(p - 90 + 8))

				case p >= 100 && p <= 107:
					bc = bc.SetColor(Color(p - 100 + 8))

				default:
					return false
				}

				screen.setColors(fc, bc)
			}

		case 'K': // Erase
			switch {
			case len(params) == 0 || params[0] == 0: // Erase to end of line
				screen := e.currentScreen()
				screen.eraseRegion(Region{
					X:  screen.cursorPos.X,
					Y:  screen.cursorPos.Y,
					X2: screen.size.X,
					Y2: screen.cursorPos.Y + 1,
				}, CRClear)
			case params[0] == 1: // Erase to start of line
				screen := e.currentScreen()
				screen.eraseRegion(Region{
					X:  0,
					Y:  screen.cursorPos.Y,
					X2: screen.cursorPos.X,
					Y2: screen.cursorPos.Y + 1,
				}, CRClear)
			case params[0] == 2: // Erase entire line
				screen := e.currentScreen()
				screen.eraseRegion(Region{
					X:  0,
					Y:  screen.cursorPos.Y,
					X2: screen.size.X,
					Y2: screen.cursorPos.Y + 1,
				}, CRClear)
			default:
				return false
			}

		case 'J': // Erase Lines
			switch {
			case len(params) == 0 || params[0] == 0: // Erase to bottom of screen
				screen := e.currentScreen()
				screen.eraseRegion(Region{
					X:  0,
					Y:  screen.cursorPos.Y,
					X2: screen.size.X,
					Y2: screen.size.Y,
				}, CRClear)
			case params[0] == 1: // Erase to top of screen
				screen := e.currentScreen()
				screen.eraseRegion(Region{
					X:  0,
					Y:  0,
					X2: screen.size.X,
					Y2: screen.cursorPos.Y,
				}, CRClear)
			case params[0] == 2: // Erase screen and home cursor
				screen := e.currentScreen()
				screen.eraseRegion(Region{
					X:  0,
					Y:  0,
					X2: screen.size.X,
					Y2: screen.size.Y,
				}, CRClear)
				screen.setCursorPos(0, 0)
			default:
				return false
			}

		case 'L': // Insert lines, scroll down
			if len(params) == 0 {
				params = []int{1}
			}
			screen := e.currentScreen()
			screen.scroll(screen.cursorPos.Y, screen.bottomMargin, params[0])

		case 'M': // Delete lines, scroll up
			if len(params) == 0 {
				params = []int{1}
			}
			screen := e.currentScreen()
			screen.scroll(screen.cursorPos.Y, screen.bottomMargin, -params[0])

		case 'S': // Scroll up
			if len(params) == 0 {
				params = []int{1}
			}
			screen := e.currentScreen()
			screen.scroll(screen.topMargin, screen.bottomMargin, -params[0])

		case 'T': // Scroll down
			if len(params) == 0 {
				params = []int{1}
			}
			screen := e.currentScreen()
			screen.scroll(screen.topMargin, screen.bottomMargin, params[0])

		case 'P': // Delete n characters
			if len(params) == 0 {
				params = []int{1}
			}
			screen := e.currentScreen()
			screen.eraseRegion(Region{
				X:  screen.cursorPos.X,
				Y:  screen.cursorPos.Y,
				X2: screen.cursorPos.X + params[0],
				Y2: screen.cursorPos.Y + 1,
			}, CRClear)

		case 'X': // Erase from cursor pos to the right
			if len(params) == 0 {
				params = []int{1}
			}
			screen := e.currentScreen()
			screen.eraseRegion(Region{
				X:  screen.cursorPos.X,
				Y:  screen.cursorPos.Y,
				X2: screen.cursorPos.X + params[0],
				Y2: screen.cursorPos.Y + 1,
			}, CRClear)

		case 'r': // Set Scroll margins
			top := 1
			bottom := e.currentScreen().size.Y
			if len(params) >= 1 {
				top = params[0]
			}
			if len(params) >= 2 {
				bottom = params[1]
			}
			e.currentScreen().setScrollMarginTopBottom(top-1, bottom-1)

		case 'n': // Device Status Report

		default:
			return false
		}
	} else if string(prefix) == "?" {
		switch b {
		case 'h', 'l': // h == set, l == reset  for various modes
			var value bool
			if b == 'h' {
				value = true
			}

			for _, p := range params {
				switch p {
				case 1: // Application / Normal Cursor Keys
					// TODO: Application Cursor Keys

				case 7: // Wraparound
					e.currentScreen().autoWrap = value

				case 9: // Send MouseXY on press
					if value {
						e.viewInts[VIMouseMode] = MMPress
					} else {
						e.viewInts[VIMouseMode] = MMNone
					}

				case 12: // Blink Cursor
					e.viewFlags[VFBlinkCursor] = value

				case 25: // Show Cursor
					e.viewFlags[VFShowCursor] = value

				case 1000: // Send MouseXY on press/release
					if value {
						e.viewInts[VIMouseMode] = MMPressRelease
					} else {
						e.viewInts[VIMouseMode] = MMNone
					}

				case 1002: // Cell Motion Mouse Tracking
					if value {
						e.viewInts[VIMouseMode] = MMPressReleaseMove
					} else {
						e.viewInts[VIMouseMode] = MMNone
					}

				case 1003: // All Motion Mouse Tracking
					if value {
						e.viewInts[VIMouseMode] = MMPressReleaseMoveAll
					} else {
						e.viewInts[VIMouseMode] = MMNone
					}

				case 1004: // Report focus changed
					e.viewFlags[VFReportFocus] = value

				case 1005: // xterm UTF-8 extended mouse reporting
					if value {
						e.viewInts[VIMouseEncoding] = MEUTF8
					} else {
						e.viewInts[VIMouseEncoding] = MEX10
					}

				case 1006: // xterm SGR extended mouse reporting
					if value {
						e.viewInts[VIMouseEncoding] = MESGR
					} else {
						e.viewInts[VIMouseEncoding] = MEX10
					}

				case 1034:
					// TODO: Interpret Meta key

				case 1049: // Save/Restore cursor and alternate screen
					e.switchScreen()

				case 2004: // Bracketed paste
					e.viewFlags[VFBracketedPaste] = value

				default:
					// TODO: Unhandled flag
				}
			}

		default:
			return false
		}
	} else if string(prefix) == ">" {
		switch b {
		case 'c': // Send Device Attributes
			attrs := "\x1b[>1;4402;0c"
			e.pty.WriteString(attrs)

		default:
			return false
		}
	} else {
		return false
	}

	return true
}

func (e *Emulator) handleCmdOSC(r *dupReader) bool {
	paramBytes := []byte{}
	var err error
	var b rune
	for {
		b, _, err = r.ReadRune()
		if err != nil {
			return false
		}

		if b >= '0' && b <= '9' {
			paramBytes = append(paramBytes, byte(b))
		} else {
			break
		}
	}

	param, _ := strconv.Atoi(string(paramBytes))

	param2 := []byte{}

	if b == ';' {
		// skip the ';'
		for {
			b, _, err = r.ReadRune()
			if err != nil {
				return false
			}
			if b == 7 || b == 0x9c { // BEL , ST
				break
			}
			if len(param2) > 0 && param2[len(param2)-1] == 27 && b == '\\' { // ESC \ is also ST
				param2 = param2[:len(param2)-1]
				break
			}

			param2 = append(param2, byte(b))
		}
	} else if b != 7 && b != 0x9c { // BEL, ST
		return false
	}

	switch param {
	case 0:
		e.viewStrings[VSWindowTitle] = string(param2)

	case 2:
		e.viewStrings[VSWindowTitle] = string(param2)

	case 4:
		// TODO: change color

	case 6:
		e.viewStrings[VSCurrentDirectory] = string(param2)

	case 7:
		e.viewStrings[VSCurrentFile] = string(param2)

	case 104:
		// TODO: Reset Color Palette

	case 112:
		// TODO: Reset Cursor Color

	default:
		return false
	}

	return true
}
