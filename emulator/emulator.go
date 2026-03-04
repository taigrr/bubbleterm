package emulator

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
	"github.com/google/uuid"
)

// Emulator is a headless terminal emulator that maintains internal state
// and renders to a framebuffer instead of directly to screen
type Emulator struct {
	mu sync.RWMutex
	id string

	// Terminal emulator (using charm's x/vt)
	vt *vt.Emulator

	// PTY for process communication
	pty, tty *os.File

	// Pipe-based I/O (alternative to PTY)
	reader io.Reader
	writer io.WriteCloser
	isPipe bool

	// Process tracking
	cmd           *exec.Cmd
	processExited bool
	onExit        func(string) // Callback when process exits, receives emulator ID

	// Framerate control
	frameRate time.Duration
	stopChan  chan struct{}

	// Damage tracking for change detection
	lastRender string
	damaged    bool

	// Screen dimensions
	width, height int
}

// EmittedFrame represents a rendered frame from the terminal.
type EmittedFrame struct {
	Rows   []string     // Each row is a string with ANSI escape codes embedded
	Damage []LineDamage // Lines that changed since the last GetScreen call
}

// New creates a new headless terminal emulator
func New(cols, rows int) (*Emulator, error) {
	e := &Emulator{
		vt:        vt.NewEmulator(cols, rows),
		id:        uuid.New().String(),
		frameRate: time.Second / 30, // Default 30 FPS
		stopChan:  make(chan struct{}),
		width:     cols,
		height:    rows,
		damaged:   true, // Initial render needed
	}

	var err error
	e.pty, e.tty, err = pty.Open()
	if err != nil {
		return nil, err
	}

	// Set initial size
	err = e.resize(cols, rows)
	if err != nil {
		return nil, err
	}

	// Start the PTY read loop
	go e.ptyReadLoop()

	return e, nil
}

// NewFromPipes creates a headless terminal emulator that reads output from r
// and writes input to w, instead of using a PTY. This is useful when the
// process is already running and you have access to its stdin/stdout pipes.
// The caller is responsible for closing the reader when the process exits.
func NewFromPipes(cols, rows int, r io.Reader, w io.WriteCloser) (*Emulator, error) {
	e := &Emulator{
		vt:        vt.NewEmulator(cols, rows),
		id:        uuid.New().String(),
		frameRate: time.Second / 30,
		stopChan:  make(chan struct{}),
		reader:    r,
		writer:    w,
		isPipe:    true,
		width:     cols,
		height:    rows,
		damaged:   true,
	}

	// Start the read loop using the provided reader
	go e.ptyReadLoop()

	return e, nil
}

func (e *Emulator) ID() string {
	return e.id
}

// SetSize sets the terminal size (same as Resize for now)
func (e *Emulator) SetSize(cols, rows int) error {
	return e.Resize(cols, rows)
}

// Resize changes the terminal dimensions
func (e *Emulator) Resize(cols, rows int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.resize(cols, rows)
}

func (e *Emulator) resize(cols, rows int) error {
	if !e.isPipe {
		err := pty.Setsize(e.pty, &pty.Winsize{
			Rows: uint16(rows),
			Cols: uint16(cols),
			X:    uint16(cols * 8),
			Y:    uint16(rows * 16),
		})
		if err != nil {
			return err
		}
	}

	e.vt.Resize(cols, rows)
	e.width = cols
	e.height = rows
	e.damaged = true

	return nil
}

// SetFrameRate sets the internal render loop framerate
func (e *Emulator) SetFrameRate(fps int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.frameRate = time.Second / time.Duration(fps)
}

// GetScreen returns the current rendered screen as ANSI strings.
// It also returns damage information about which lines changed since
// the last call.
func (e *Emulator) GetScreen() EmittedFrame {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Render the current screen
	rendered := e.vt.Render()

	// Check for changes
	var damage []LineDamage
	if rendered != e.lastRender || e.damaged {
		// Mark all lines as damaged for simplicity
		// (the vt package tracks touched lines but we simplify here)
		for y := 0; y < e.height; y++ {
			damage = append(damage, LineDamage{
				Row:    y,
				X1:     0,
				X2:     e.width,
				Reason: CRText,
			})
		}
		e.lastRender = rendered
		e.damaged = false
	}

	// Split rendered output into rows
	rows := splitIntoRows(rendered, e.height, e.width)

	return EmittedFrame{Rows: rows, Damage: damage}
}

// splitIntoRows splits the rendered output into individual rows and pads to width
func splitIntoRows(rendered string, height, width int) []string {
	rows := make([]string, height)

	// The vt.Render() returns a string with ANSI codes
	// We need to split it by newlines while preserving ANSI codes
	currentRow := 0
	var currentLine string

	for _, r := range rendered {
		if r == '\n' {
			if currentRow < height {
				rows[currentRow] = padRow(currentLine, width)
				currentRow++
			}
			currentLine = ""
		} else {
			currentLine += string(r)
		}
	}

	// Handle last line if no trailing newline
	if currentRow < height && currentLine != "" {
		rows[currentRow] = padRow(currentLine, width)
		currentRow++
	}

	// Fill remaining rows with spaces
	emptyRow := strings.Repeat(" ", width)
	for i := currentRow; i < height; i++ {
		if rows[i] == "" {
			rows[i] = emptyRow
		}
	}

	return rows
}

// padRow pads a row to the specified width, accounting for ANSI escape codes
func padRow(row string, width int) string {
	// Count visible characters (ignoring ANSI escape codes)
	visibleLen := 0
	inEscape := false
	for _, r := range row {
		if r == '\033' {
			inEscape = true
		} else if inEscape {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '~' {
				inEscape = false
			}
		} else {
			visibleLen++
		}
	}

	// Pad with spaces if needed
	if visibleLen < width {
		return row + strings.Repeat(" ", width-visibleLen)
	}
	return row
}

// Cursor returns the current cursor position and whether the cursor is visible.
func (e *Emulator) Cursor() (Pos, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	pos := e.vt.CursorPosition()
	// The vt package doesn't expose cursor visibility directly in a simple way
	// Default to visible
	return Pos{X: pos.X, Y: pos.Y}, true
}

// FeedInput processes raw ANSI input (typically from PTY)
func (e *Emulator) FeedInput(data []byte) {
	// This will be called by the PTY read loop
	// For now, we don't need to expose this publicly since PTY handles it
}

// SetOnExit sets a callback function that will be called when the process exits
func (e *Emulator) SetOnExit(callback func(string)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onExit = callback
}

// IsProcessExited returns true if the process has exited
func (e *Emulator) IsProcessExited() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.processExited
}

// StartCommand starts a command in the terminal.
// This is not supported for pipe-based emulators; use NewFromPipes instead.
func (e *Emulator) StartCommand(cmd *exec.Cmd) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.isPipe {
		return fmt.Errorf("StartCommand is not supported on pipe-based emulators")
	}

	if e.pty == nil {
		return ErrPTYNotInitialized
	}

	// Set up environment
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}

	// Ensure TERM is set correctly
	termSet := false
	for i, env := range cmd.Env {
		if len(env) >= 5 && env[:5] == "TERM=" {
			cmd.Env[i] = "TERM=xterm-256color"
			termSet = true
			break
		}
	}
	if !termSet {
		cmd.Env = append(cmd.Env, "TERM=xterm-256color")
	}

	// Connect to PTY
	cmd.Stdout = e.tty
	cmd.Stdin = e.tty
	cmd.Stderr = e.tty

	// Set up process group for proper signal handling
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setctty = true
	cmd.SysProcAttr.Setsid = true
	// Don't set Ctty explicitly - let the system handle it

	// Store the command reference
	e.cmd = cmd
	e.processExited = false

	err := cmd.Start()
	if err != nil {
		return err
	}

	// Start monitoring the process in a goroutine
	go e.monitorProcess()

	return nil
}

// monitorProcess waits for the process to exit and calls the exit callback
func (e *Emulator) monitorProcess() {
	if e.cmd == nil {
		return
	}

	// Wait for the process to exit
	_ = e.cmd.Wait()

	e.mu.Lock()
	e.processExited = true
	onExit := e.onExit
	id := e.id
	e.mu.Unlock()

	// Call the exit callback if set
	if onExit != nil {
		onExit(id)
	}
}

// Write sends data to the PTY or pipe (keyboard input)
func (e *Emulator) Write(data []byte) (int, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.isPipe {
		if e.writer == nil {
			return 0, ErrPTYNotInitialized
		}
		return e.writer.Write(data)
	}

	if e.pty == nil {
		return 0, ErrPTYNotInitialized
	}

	return e.pty.Write(data)
}

// SendKey sends a key event to the terminal
func (e *Emulator) SendKey(key string) error {
	_, err := e.Write([]byte(key))
	return err
}

// SendMouse sends a mouse event to the terminal in SGR format
func (e *Emulator) SendMouse(button int, x, y int, pressed bool) error {
	// Convert to the vt package's mouse event format
	var vtButton vt.MouseButton
	switch button {
	case 0:
		vtButton = vt.MouseLeft
	case 1:
		vtButton = vt.MouseMiddle
	case 2:
		vtButton = vt.MouseRight
	case -1:
		vtButton = vt.MouseNone // Motion
	default:
		vtButton = vt.MouseButton(button)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if pressed {
		e.vt.SendMouse(vt.MouseClick{
			Button: vtButton,
			X:      x,
			Y:      y,
		})
	} else if button == -1 {
		e.vt.SendMouse(vt.MouseMotion{
			Button: vtButton,
			X:      x,
			Y:      y,
		})
	} else {
		e.vt.SendMouse(vt.MouseRelease{
			Button: vtButton,
			X:      x,
			Y:      y,
		})
	}

	return nil
}

// Close shuts down the emulator
func (e *Emulator) Close() error {
	close(e.stopChan)

	if e.isPipe {
		if e.writer != nil {
			e.writer.Close()
		}
		return nil
	}

	if e.tty != nil {
		e.tty.Close()
	}
	if e.pty != nil {
		e.pty.Close()
	}

	return e.vt.Close()
}

// ptyReadLoop reads from PTY/pipe and writes to the vt emulator
func (e *Emulator) ptyReadLoop() {
	var source io.Reader
	if e.isPipe {
		source = e.reader
	} else {
		source = e.pty
	}

	buf := make([]byte, 4096)
	for {
		select {
		case <-e.stopChan:
			return
		default:
		}

		n, err := source.Read(buf)
		if err != nil {
			return
		}

		if n > 0 {
			e.mu.Lock()
			e.vt.Write(buf[:n])
			e.damaged = true
			e.mu.Unlock()
		}
	}
}
