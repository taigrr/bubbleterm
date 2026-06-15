package bubbleterm

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
)

// keyToTerminalInput converts bubbletea key messages to terminal input byte sequences.
// It uses structured key fields (Code, Mod, Text) instead of string matching
// to handle all key combinations programmatically.
func keyToTerminalInput(msg tea.KeyMsg) string {
	k := msg.Key()
	mod := k.Mod & (tea.ModShift | tea.ModAlt | tea.ModCtrl)

	switch k.Code {
	case tea.KeyEnter:
		if mod&tea.ModAlt != 0 {
			return "\x1b\x0a"
		}
		if mod != 0 {
			return "\x0a"
		}
		return "\r"
	case tea.KeyTab:
		if mod&tea.ModShift != 0 {
			return "\x1b[Z"
		}
		return "\t"
	case tea.KeyBackspace:
		if mod&tea.ModAlt != 0 {
			return "\x1b\x7f"
		}
		if mod&tea.ModCtrl != 0 {
			return "\x08"
		}
		return "\x7f"
	case tea.KeyEscape:
		return "\x1b"
	case tea.KeySpace:
		if mod&tea.ModCtrl != 0 {
			if mod&tea.ModAlt != 0 {
				return "\x1b\x00"
			}
			return "\x00"
		}
		if mod&tea.ModAlt != 0 {
			return "\x1b "
		}
		return " "
	case tea.KeyUp:
		return csiLetter('A', mod)
	case tea.KeyDown:
		return csiLetter('B', mod)
	case tea.KeyRight:
		return csiLetter('C', mod)
	case tea.KeyLeft:
		return csiLetter('D', mod)
	case tea.KeyHome:
		return csiLetter('H', mod)
	case tea.KeyEnd:
		return csiLetter('F', mod)
	case tea.KeyInsert:
		return csiTilde(2, mod)
	case tea.KeyDelete:
		return csiTilde(3, mod)
	case tea.KeyPgUp:
		return csiTilde(5, mod)
	case tea.KeyPgDown:
		return csiTilde(6, mod)
	case tea.KeyF1:
		return ss3Func('P', mod)
	case tea.KeyF2:
		return ss3Func('Q', mod)
	case tea.KeyF3:
		return ss3Func('R', mod)
	case tea.KeyF4:
		return ss3Func('S', mod)
	case tea.KeyF5:
		return csiTilde(15, mod)
	case tea.KeyF6:
		return csiTilde(17, mod)
	case tea.KeyF7:
		return csiTilde(18, mod)
	case tea.KeyF8:
		return csiTilde(19, mod)
	case tea.KeyF9:
		return csiTilde(20, mod)
	case tea.KeyF10:
		return csiTilde(21, mod)
	case tea.KeyF11:
		return csiTilde(23, mod)
	case tea.KeyF12:
		return csiTilde(24, mod)
	}

	// Printable text (covers regular characters, shift+letter producing uppercase, symbols)
	if k.Text != "" {
		if mod&tea.ModAlt != 0 {
			return "\x1b" + k.Text
		}
		return k.Text
	}

	// Ctrl+letter: ctrl+a=\x01 .. ctrl+z=\x1a
	if mod&tea.ModCtrl != 0 && k.Code >= 'a' && k.Code <= 'z' {
		ch := string(rune(k.Code - 'a' + 1))
		if mod&tea.ModAlt != 0 {
			return "\x1b" + ch
		}
		return ch
	}

	// Ctrl+symbol
	if mod&tea.ModCtrl != 0 {
		if c, ok := ctrlSymbol(k.Code); ok {
			ch := string(c)
			if mod&tea.ModAlt != 0 {
				return "\x1b" + ch
			}
			return ch
		}
	}

	// Alt+printable (no Text field set)
	if mod&tea.ModAlt != 0 && k.Code >= 0x20 && k.Code <= 0x7e {
		return "\x1b" + string(k.Code)
	}

	// Plain printable rune fallback
	if k.Code >= 0x20 && k.Code <= 0x7e {
		return string(k.Code)
	}

	return ""
}

// ctrlSymbol maps punctuation to their ctrl-modified byte.
func ctrlSymbol(code rune) (rune, bool) {
	switch code {
	case '@':
		return 0x00, true
	case '[':
		return 0x1b, true
	case '\\':
		return 0x1c, true
	case ']':
		return 0x1d, true
	case '^':
		return 0x1e, true
	case '_':
		return 0x1f, true
	}
	return 0, false
}

// modParam returns the xterm modifier parameter for the given modifiers.
// xterm encodes: 1 + (shift?1:0) + (alt?2:0) + (ctrl?4:0)
func modParam(mod tea.KeyMod) int {
	p := 1
	if mod&tea.ModShift != 0 {
		p += 1
	}
	if mod&tea.ModAlt != 0 {
		p += 2
	}
	if mod&tea.ModCtrl != 0 {
		p += 4
	}
	return p
}

// csiLetter returns CSI sequences for arrow/home/end keys.
// Plain: \x1b[X, Modified: \x1b[1;{param}X
func csiLetter(final byte, mod tea.KeyMod) string {
	if mod == 0 {
		return "\x1b[" + string(final)
	}
	return "\x1b[1;" + strconv.Itoa(modParam(mod)) + string(final)
}

// csiTilde returns CSI tilde sequences for insert/delete/pgup/pgdown/function keys.
// Plain: \x1b[N~, Modified: \x1b[N;{param}~
func csiTilde(n int, mod tea.KeyMod) string {
	ns := strconv.Itoa(n)
	if mod == 0 {
		return "\x1b[" + ns + "~"
	}
	return "\x1b[" + ns + ";" + strconv.Itoa(modParam(mod)) + "~"
}

// ss3Func returns SS3 sequences for F1-F4.
// Plain: \x1bOX, Modified: \x1b[1;{param}X
func ss3Func(final byte, mod tea.KeyMod) string {
	if mod == 0 {
		return "\x1bO" + string(final)
	}
	return "\x1b[1;" + strconv.Itoa(modParam(mod)) + string(final)
}
