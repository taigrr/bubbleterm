package emulator

import (
	"fmt"
	"image/color"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
	"github.com/google/uuid"
)

// cursorStyleFromVT maps upstream vt.CursorStyle to our CursorStyle.
// An explicit switch avoids a silent int cast that would break if the
// upstream iota order ever changes.
func cursorStyleFromVT(s vt.CursorStyle) CursorStyle {
	switch s {
	case vt.CursorUnderline:
		return CursorUnderline
	case vt.CursorBar:
		return CursorBar
	default:
		return CursorBlock
	}
}

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

	closeOnce sync.Once

	// Process tracking
	cmd           *exec.Cmd
	processExited bool
	onExit        func(string) // Callback when process exits, receives emulator ID

	stopChan chan struct{}

	// Damage tracking for change detection
	lastRender string
	lastRows   []string
	damaged    bool
	notifyC    chan struct{} // signaled when new damage occurs

	// Screen dimensions
	width, height int

	// Cursor state tracked via vt callbacks. These fields are written inside
	// e.vt.Write calls, which ptyReadLoop always makes under e.mu.Lock.
	// Readers use e.mu.RLock as normal.
	cursorHidden bool
	cursorStyle  vt.CursorStyle
	cursorSteady bool // true = not blinking; zero value (false) = blinking, matching DEC default
	cursorColor  color.Color
}

// EmittedFrame represents a rendered frame from the terminal.
type EmittedFrame struct {
	Rows   []string     // Each row is a string with ANSI escape codes embedded
	Damage []LineDamage // Lines that changed since the last GetScreen call
}

// newEmulator creates a base Emulator with common fields.
func newEmulator(cols, rows int) *Emulator {
	e := &Emulator{
		vt:       vt.NewEmulator(cols, rows),
		id:       uuid.New().String(),
		stopChan: make(chan struct{}),
		notifyC:  make(chan struct{}, 1),
		width:    cols,
		height:   rows,
		damaged:  true, // Initial render needed
	}
	e.lastRows = splitIntoRows("", cols, rows)
	e.vt.SetCallbacks(vt.Callbacks{
		CursorVisibility: func(visible bool) { e.cursorHidden = !visible },
		// NOTE: upstream declares this parameter as "blink" but actually
		// passes !blink (steady). See charmbracelet/x/vt screen.go:251.
		CursorStyle: func(style vt.CursorStyle, steady bool) {
			e.cursorStyle = style
			e.cursorSteady = steady
		},
		CursorColor: func(c color.Color) { e.cursorColor = c },
	})
	return e
}

// New creates a new headless terminal emulator
func New(cols, rows int) (*Emulator, error) {
	e := newEmulator(cols, rows)

	var err error
	e.pty, e.tty, err = pty.Open()
	if err != nil {
		return nil, err
	}

	// Set initial size
	err = e.resize(cols, rows)
	if err != nil {
		e.pty.Close()
		e.tty.Close()
		return nil, err
	}

	// Start the PTY read loop and drain terminal responses
	// (DA/DSR/XTVERSION/in-band-resize) back to the child so capability
	// queries don't stall before rendering.
	go e.ptyReadLoop()
	go e.responseLoop()

	return e, nil
}

// NewFromPipes creates a headless terminal emulator that reads output from r
// and writes input to w, instead of using a PTY. This is useful when the
// process is already running and you have access to its stdin/stdout pipes.
// The caller is responsible for closing the reader when the process exits.
func NewFromPipes(cols, rows int, r io.Reader, w io.WriteCloser) (*Emulator, error) {
	e := newEmulator(cols, rows)
	e.reader = r
	e.writer = w
	e.isPipe = true

	// Start the read loop using the provided reader and drain terminal
	// responses (for queries like DA/DSR) back to the remote process.
	go e.responseLoop()
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
	e.markDamaged()

	return nil
}

// markDamaged sets the damaged flag and signals notifyC.
// Must be called with mu held.
func (e *Emulator) markDamaged() {
	e.damaged = true
	select {
	case e.notifyC <- struct{}{}:
	default:
	}
}

// GetScreen returns the current rendered screen as ANSI strings.
// It also returns damage information about which lines changed since
// the last call. When nothing has changed since the last call, it
// returns cached rows with empty Damage.
func (e *Emulator) GetScreen() EmittedFrame {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.damaged {
		return EmittedFrame{Rows: e.lastRows}
	}

	rendered := e.vt.Render()
	e.damaged = false

	// Check for changes
	var damage []LineDamage
	if rendered != e.lastRender {
		damage = make([]LineDamage, e.height)
		for y := 0; y < e.height; y++ {
			damage[y] = LineDamage{
				Row:    y,
				X1:     0,
				X2:     e.width,
				Reason: CRText,
			}
		}
		e.lastRender = rendered
	}

	rows := splitIntoRows(rendered, e.height, e.width)
	e.lastRows = rows
	return EmittedFrame{Rows: rows, Damage: damage}
}

// splitIntoRows splits the rendered output into individual rows and pads to width
func splitIntoRows(rendered string, height, width int) []string {
	rows := make([]string, height)
	lines := strings.Split(rendered, "\n")
	emptyRow := strings.Repeat(" ", width)

	for i := range height {
		if i < len(lines) && lines[i] != "" {
			rows[i] = padRow(lines[i], width)
		} else {
			rows[i] = emptyRow
		}
	}

	return rows
}

// padRow pads a row to the specified width, accounting for ANSI escape codes.
// It always appends a SGR reset (\033[0m) before any trailing spaces so that
// active attributes (e.g. underline, bold) from the row's content do not bleed
// into the padding or into subsequent rows when rows are joined with \n.
func padRow(row string, width int) string {
	const reset = "\033[0m"
	if visibleLen := ansi.StringWidth(row); visibleLen < width {
		return row + reset + strings.Repeat(" ", width-visibleLen)
	}
	return row + reset
}

// CellAt returns the cell at the given column (x) and row (y).
// It returns nil if the position is out of bounds.
func (e *Emulator) CellAt(x, y int) *uv.Cell {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.vt.CellAt(x, y)
}

// GetCells returns the full screen buffer as a 2D grid of cells.
// The outer slice is rows, the inner slice is columns.
func (e *Emulator) GetCells() [][]uv.Cell {
	e.mu.RLock()
	defer e.mu.RUnlock()
	// TODO: replace per-cell loop with vt.Line(y) once charmbracelet/x/vt
	// exposes it (Screen.buf is private).
	cells := make([][]uv.Cell, e.height)
	for y := range e.height {
		cells[y] = make([]uv.Cell, e.width)
		for x := range e.width {
			if c := e.vt.CellAt(x, y); c != nil {
				cells[y][x] = *c
			}
		}
	}
	return cells
}

// Cursor returns the current cursor position and whether the cursor is visible.
func (e *Emulator) Cursor() (Pos, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	pos := e.vt.CursorPosition()
	return Pos{X: pos.X, Y: pos.Y}, !e.cursorHidden
}

// CursorAppearance returns the current cursor shape, blink state, and color
// as set by the running application via DECSCUSR and OSC 12.
func (e *Emulator) CursorAppearance() CursorAppearance {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return CursorAppearance{
		Style: cursorStyleFromVT(e.cursorStyle),
		Blink: !e.cursorSteady,
		Color: e.cursorColor,
	}
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

// SendMouseWheel sends a mouse wheel event to the terminal
func (e *Emulator) SendMouseWheel(button int, x, y int) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.vt.SendMouse(vt.MouseWheel{
		Button: vt.MouseButton(button),
		X:      x,
		Y:      y,
	})

	return nil
}

// Close shuts down the emulator
func (e *Emulator) Close() error {
	var closeErr error

	e.closeOnce.Do(func() {
		close(e.stopChan)

		if e.isPipe {
			// Leave the vt emulator open here. Its Close method races with
			// concurrent Read/Write calls, and the pipe/PTY closures are enough to
			// stop the goroutines that feed it.
			if e.writer != nil {
				if err := e.writer.Close(); err != nil && closeErr == nil {
					closeErr = err
				}
			}
			return
		}

		if e.tty != nil {
			if err := e.tty.Close(); err != nil && closeErr == nil {
				closeErr = err
			}
		}
		if e.pty != nil {
			if err := e.pty.Close(); err != nil && closeErr == nil {
				closeErr = err
			}
		}
		// Intentionally do not call e.vt.Close(); the upstream vt emulator does
		// not synchronize Close with Read/Write, and we already stop further I/O
		// by closing the pipe/PTY endpoints above.
	})

	return closeErr
}

// Done returns a channel that is closed when the emulator is closed. Callers
// blocked on polling (e.g. an auto-poll loop) can select on this to exit
// promptly instead of sleeping until process exit.
func (e *Emulator) Done() <-chan struct{} {
	return e.stopChan
}

// NotifyChanged returns a channel that receives a value each time the
// emulator's screen content changes. The channel is buffered (size 1) so
// a single pending notification is coalesced when multiple writes arrive
// before the consumer reads.
func (e *Emulator) NotifyChanged() <-chan struct{} {
	return e.notifyC
}

// responseLoop forwards responses the vt emulator generates for terminal
// queries (DA/DSR/XTVERSION/in-band-resize) back to the child process, i.e.
// onto its stdin. The underlying vt writes these to a synchronous io.Pipe, so
// without draining them the write blocks the read loop under e.mu and freezes
// the terminal. It targets the PTY master or the pipe writer depending on how
// the emulator was constructed.
func (e *Emulator) responseLoop() {
	var dst io.Writer
	if e.isPipe {
		dst = e.writer
	} else {
		dst = e.pty
	}

	buf := make([]byte, 4096)
	for {
		select {
		case <-e.stopChan:
			return
		default:
		}
		n, err := e.vt.Read(buf)
		if n > 0 && dst != nil {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
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
			e.markDamaged()
			e.mu.Unlock()
		}
	}
}
