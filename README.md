# Headless Terminal Emulator in Go

A fully-functional, headless, embeddable terminal emulator written in Golang. This library focuses on **terminal emulation** - parsing ANSI escape sequences, maintaining screen state, and rendering frames. It's designed to work with PTY libraries like [`creack/pty`](https://github.com/creack/pty) for complete terminal functionality.

## 💡 Goals

This library provides the **terminal emulation layer** that sits between PTY I/O and your application. It can:

- Parse and interpret ANSI escape sequences (CSI, OSC, ESC, DCS)
- Maintain terminal screen state (cursor position, colors, attributes)
- Handle 256-color and true color (24-bit RGB) rendering
- Support alternate screen buffers and scrollback
- Process keyboard and mouse input events
- Render frames as ANSI-preserved strings for TUI frameworks
- Emulate `$TERM = xterm-256color` behavior accurately
- Integrate seamlessly with PTY libraries like [`creack/pty`](https://github.com/creack/pty)

## 📦 Features

| Feature | Status |
|---------|--------|
| ANSI parser (CSI, OSC) | ✅ Core complete |
| UTF-8 support | ✅ |
| Text attributes (bold, underline, etc) | ✅ |
| 256-color + true color | ✅ |
| Cursor & scrollback | ✅ |
| Mouse input (SGR mode) | ✅ In progress |
| Keyboard input support | ✅ |
| Resize support | ✅ |
| $TERM compatibility | ✅ xterm-256color |
| Bubbletea-compatible output | ✅ |
| htop rendering | 🟡 Needs validation |
| Adjustable frame rate | ✅ |
| Process termination API | ✅ |

## 📐 Architecture

```
TerminalEmulator
├── FeedInput([]byte)        // Raw ANSI input from PTY
├── SendKey(KeyEvent)        // Simulate keyboard input
├── SendMouse(MouseEvent)    // Simulate mouse input
├── RenderFrame() EmittedFrame // Get rendered screen as ANSI strings
├── Resize(w, h int)         // Change screen dimensions
├── SetFrameRate(fps int)    // Control render loop timing
└── TermName() string        // Get $TERM ("xterm-256color")
```

### Integration with PTY

This library is designed to work with PTY libraries:

```go
// Create PTY with creack/pty
pty, tty, _ := pty.Open()
cmd := exec.Command("htop")
cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
cmd.Start()

// Create terminal emulator
term := NewTerminalEmulator(80, 24)

// Connect PTY output to terminal emulator
go func() {
    buf := make([]byte, 1024)
    for {
        n, _ := pty.Read(buf)
        term.FeedInput(buf[:n])  // Parse ANSI and update screen state
    }
}()

// Send input from terminal emulator to PTY
go func() {
    // Handle keyboard/mouse events and write to pty
}()
```

## 🖼️ EmittedFrame Output

The `RenderFrame()` method returns:

```go
type EmittedFrame struct {
    Rows []string // Each row is a string with ANSI escape codes embedded
}
```

This lets you do:

```go
for _, row := range emulator.RenderFrame().Rows {
    fmt.Println(row)
}
```

Or integrate into your Bubbletea `View()`.

## 🧪 htop Support Requirements

To support htop, your emulator must:

- Handle 256-color + RGB escape sequences
- Track and wrap the cursor correctly
- Maintain scroll regions
- Interpret full ANSI/VT escape sequences (at least CSI, OSC, ESC, DCS)
- Support alternate screen buffer (`\x1b[?1049h`)
- Correctly track terminal resize events
- React to mouse mode enable/disable sequences:
  - `\x1b[?1000h` – mouse click tracking
  - `\x1b[?1006h` – SGR extended mouse reporting
- Emit output line-by-line in bubbletea-compatible ANSI strings

## 🚀 Getting Started

```bash
go get github.com/yourname/go-headless-terminal
go get github.com/creack/pty
```

### Basic Example with PTY Integration

```go
package main

import (
    "fmt"
    "os"
    "os/exec"
    "time"
    
    "github.com/creack/pty"
    "github.com/yourname/go-headless-terminal"
)

func main() {
    // Create PTY and launch process
    cmd := exec.Command("htop")
    cmd.Env = append(os.Environ(), "TERM=xterm-256color")
    
    ptyFile, err := pty.Start(cmd)
    if err != nil {
        panic(err)
    }
    defer ptyFile.Close()
    
    // Create terminal emulator
    term := NewTerminalEmulator(80, 24)
    term.SetFrameRate(10)
    
    // Feed PTY output to terminal emulator
    go func() {
        buf := make([]byte, 1024)
        for {
            n, err := ptyFile.Read(buf)
            if err != nil {
                return
            }
            term.FeedInput(buf[:n])
        }
    }()
    
    // Render terminal frames
    for {
        frame := term.RenderFrame()
        // Clear screen and render
        fmt.Print("\033[2J\033[H")
        for _, row := range frame.Rows {
            fmt.Println(row)
        }
        time.Sleep(time.Second / 10)
    }
}
```

### Standalone Usage (Without PTY)

```go
func main() {
    term := NewTerminalEmulator(80, 24)
    
    // Simulate ANSI input
    term.FeedInput([]byte("\033[31mHello \033[32mWorld\033[0m\n"))
    term.FeedInput([]byte("Line 2\n"))
    
    // Render frame
    frame := term.RenderFrame()
    for _, row := range frame.Rows {
        fmt.Println(row)
    }
}
```

## 🧠 Tips and Advice

### ✅ Parse ANSI first, render second

Avoid baking styles into your data layer. Use a `Cell{Rune, Style}` representation and only emit SGR codes during rendering.

### ✅ Treat input and output as decoupled

Read from pty, interpret into state. Push input with `SendKey()` and `SendMouse()`.

### ✅ Normalize all colors to RGB internally

Even if a code uses 256-color, normalize it to `Color{R,G,B}` to simplify rendering.

### ✅ Track alternate screen buffer

Many programs (like htop, vim) switch to an alternate screen and expect it to be cleared/restored.

### ✅ Avoid rendering when nothing changed (optional)

Track dirty lines or hash screen state to avoid redundant re-draws.

## 📚 Resources

- [creack/pty](https://github.com/creack/pty) - PTY interface for Go (recommended companion library)
- [XTerm Control Sequences](https://invisible-island.net/xterm/ctlseqs/ctlseqs.html)
- [VT100 / VT220 Reference](https://vt100.net/)
- [Charm Bubbletea](https://github.com/charmbracelet/bubbletea)
- [Charm Glamour (ANSI Renderer)](https://github.com/charmbracelet/glamour)

## 📜 License

MIT

## ⚙️ Roadmap

This library focuses on terminal emulation. For complete terminal functionality:

- **PTY Management**: Use [`creack/pty`](https://github.com/creack/pty) for process and PTY handling
- **Terminal Emulation**: This library handles ANSI parsing and screen rendering
- **TUI Integration**: Output works seamlessly with Bubbletea and other TUI frameworks

Contributions welcome. Let's build the best terminal emulation layer for Go!