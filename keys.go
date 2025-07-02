package bubbleterm

import (
	tea "github.com/charmbracelet/bubbletea/v2"
)

// keyToTerminalInput converts bubbletea key messages to terminal input strings
func keyToTerminalInput(msg tea.KeyMsg) string {
	// Use string matching for bubbletea v2 compatibility
	switch msg.String() {
	case "enter":
		return "\r"
	case "tab":
		return "\t"
	case "backspace":
		return "\b"
	case "delete":
		return "\x7f"
	case "esc":
		return "\x1b"
	case " ":
		return " "
	case "up":
		return "\x1b[A"
	case "down":
		return "\x1b[B"
	case "right":
		return "\x1b[C"
	case "left":
		return "\x1b[D"
	case "home":
		return "\x1b[H"
	case "end":
		return "\x1b[F"
	case "pageup":
		return "\x1b[5~"
	case "pagedown":
		return "\x1b[6~"
	case "insert":
		return "\x1b[2~"
	case "f1":
		return "\x1bOP"
	case "f2":
		return "\x1bOQ"
	case "f3":
		return "\x1bOR"
	case "f4":
		return "\x1bOS"
	case "f5":
		return "\x1b[15~"
	case "f6":
		return "\x1b[17~"
	case "f7":
		return "\x1b[18~"
	case "f8":
		return "\x1b[19~"
	case "f9":
		return "\x1b[20~"
	case "f10":
		return "\x1b[21~"
	case "f11":
		return "\x1b[23~"
	case "f12":
		return "\x1b[24~"
	case "ctrl+c":
		return "\x03"
	case "ctrl+d":
		return "\x04"
	case "ctrl+z":
		return "\x1a"
	case "ctrl+l":
		return "\x0c"
	default:
		// For regular characters, return the string as-is
		// This handles letters, numbers, symbols, etc.
		str := msg.String()
		if len(str) == 1 {
			return str
		}
		return ""
	}
}