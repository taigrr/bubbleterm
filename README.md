# Bubbleterm: A Headless Terminal Emulator in Go

A fully-functional, headless, embeddable terminal emulator written in Golang.
This library focuses on **terminal emulation** - parsing ANSI escape sequences, maintaining screen state, and rendering frames.
It's designed to work with PTY libraries like [`creack/pty`](https://github.com/creack/pty) for complete terminal functionality.
Finally, we provide a Bubbletea-compatible output format for building terminal user interfaces (TUIs).

## 💡 Goals

This library provides the **terminal emulation layer** and Bubble components that sits between PTY I/O and your application. It can:

- Parse and interpret ANSI escape sequences (CSI, OSC, ESC, DCS)
- Maintain terminal screen state (cursor position, colors, attributes)
- Handle 256-color and true color (24-bit RGB) rendering
- Support alternate screen buffers and scrollback
- Process keyboard and mouse input events
- Render frames as ANSI-preserved strings for TUI frameworks
- Emulate `$TERM = xterm-256color` behavior accurately

## 📦 Features

| Feature                                | Status            |
| -------------------------------------- | ----------------- |
| ANSI parser (CSI, OSC)                 | ✅ Core complete  |
| UTF-8 support                          | ✅                |
| Text attributes (bold, underline, etc) | ✅                |
| 256-color + true color                 | ✅                |
| Cursor & scrollback                    | ✅                |
| Keyboard input support                 | ✅                |
| Resize support                         | ✅                |
| `$TERM` compatibility                  | ✅ xterm-256color |
| Bubbletea-compatible output            | ✅                |
| Adjustable frame rate                  | ✅                |
| Process termination API                | ✅                |

## 🚀 Getting Started

```bash
go get github.com/taigrr/bubbleterm
```

## 📋 Usage Examples

This library provides three ways to use the terminal emulator:

### 1. Bubbletea Integration (`cmd/bubbleterm`)

Run a terminal application within a Bubbletea TUI:

```bash
go run cmd/bubbleterm/main.go
```

This example shows how to:

- Create a terminal bubble that runs `htop`
- Handle keyboard input (Ctrl+C/q to quit)
- Forward all messages to the terminal bubble
- Display the terminal output in a TUI

```go
// import bubbleterm "github.com/taigrr/bubbleterm"
// Create a new terminal bubble and start htop
cmd := exec.Command("htop")
terminal, err := bubbleterm.NewWithCommand(80, 24, cmd)

// Use in your Bubbletea model
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd
    terminalModel, cmd := m.terminal.Update(msg)
    m.terminal = terminalModel.(*bubbleterm.Model)
    return m, cmd
}
```

### 2. Headless Emulator (`cmd/staticprint`)

Use the terminal emulator without a TUI for programmatic access:

```bash
go run cmd/staticprint/main.go
```

This example demonstrates:

- Creating a headless terminal emulator
- Starting a command (`htop`)
- Capturing terminal output as frames
- Resizing the terminal dynamically

```go
// Create a new emulator
emu, err := emulator.New(80, 24)
defer emu.Close()

// Start a command
cmd := exec.Command("htop")
err = emu.StartCommand(cmd)

// Get the screen output
frame := emu.GetScreen()
for i, row := range frame.Rows {
    fmt.Printf("%2d: %s\n", i, row)
}

// Resize the terminal
emu.Resize(100, 40)
```

### 3. Multi-Window Terminal Manager (`cmd/multiwindow`)

A complete windowing system with multiple terminal instances:


https://github.com/user-attachments/assets/c318dcab-4152-4166-94ba-3f20e228822d


```bash
go run cmd/multiwindow/main.go
```

Features:

- **Right-click**: Create new terminal window
- **Left-click**: Select and drag windows
- **'i'**: Enter insert mode (input goes to focused terminal)
- **ESC**: Exit insert mode
- **+/-**: Resize focused window
- **Ctrl+C/q**: Quit application

This example shows advanced usage:

- Multiple terminal instances running simultaneously
- Window management with focus and z-ordering
- Mouse event translation between screen and window coordinates
- Centralized terminal updates with proper cleanup

## 🔧 Core API

### Basic Terminal Emulator

```go
// Create emulator
emu, err := emulator.New(width, height)

// Start a command
cmd := exec.Command("your-command")
emu.StartCommand(cmd)

// Get rendered output
frame := emu.GetScreen()
for _, row := range frame.Rows {
    fmt.Println(row)
}

// Resize
emu.Resize(newWidth, newHeight)

// Cleanup
emu.Close()
```

### Bubbletea Integration

```go
// Create terminal bubble
terminal, err := bubbleterm.NewWithCommand(width, height, cmd)
terminal.SetAutoPoll(false) // Disable auto-polling for updates

// In your Bubbletea model
func (m *model) Init() tea.Cmd {
    return m.terminal.Init()
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    terminalModel, cmd := m.terminal.Update(msg)
    m.terminal = terminalModel.(*bubbleterm.Model)
    return m, cmd
}

func (m *model) View() string {
    return m.terminal.View()
}
```

### Advanced Features

```go
// Focus management
terminal.Focus()
terminal.Blur()
focused := terminal.Focused()

// Manual input sending
terminal.SendInput("ls\n")

// Process monitoring
if terminal.GetEmulator().IsProcessExited() {
    // Handle process exit
}

// Auto-polling control (for custom update loops)
terminal.SetAutoPoll(false)
cmd := terminal.UpdateTerminal() // Manual poll
```

## Limitations and Known Issues

- Damage tracking is not yet implemented, so the entire screen may be redrawn on every frame
- Sometimes, character deletion (backspace) may not work as expected due to missing damage tracking
- Running tmux inside the emulator fixes these issues, as tmux handles its own damage tracking
- We may decide to use a different emulator library in the future if it provides better performance or features

## 📚 Resources

- [creack/pty](https://github.com/creack/pty) - PTY interface for Go (recommended companion library)
- [XTerm Control Sequences](https://invisible-island.net/xterm/ctlseqs/ctlseqs.html)
- [VT100 / VT220 Reference](https://vt100.net/)
- [Charm Bubbletea](https://github.com/charmbracelet/bubbletea)
- [Charm Glamour (ANSI Renderer)](https://github.com/charmbracelet/glamour)
- [Bubbletea v2 Compositing Example](https://gist.github.com/Gaurav-Gosain/f6ff73cd53b8732af452dc27a3c76f59)

## 📜 License

0BSD

## ⚙️ Roadmap

This library focuses on terminal emulation. For complete terminal functionality:

- **PTY Management**: Use [`creack/pty`](https://github.com/creack/pty) for process and PTY handling
- **Terminal Emulation**: This library handles ANSI parsing and screen rendering
- **TUI Integration**: Output works seamlessly with Bubbletea and other TUI frameworks

Contributions welcome!
