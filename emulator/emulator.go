package emulator

import (
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
)

// Emulator is a headless terminal emulator that maintains internal state
// and renders to a framebuffer instead of directly to screen
type Emulator struct {
	mu sync.RWMutex

	// Terminal state
	mainScreen  *screen
	altScreen   *screen
	onAltScreen bool

	// PTY for process communication
	pty, tty *os.File

	// Framerate control
	frameRate time.Duration
	stopChan  chan struct{}

	// Terminal settings
	viewFlags   []bool
	viewInts    []int
	viewStrings []string
}

// EmittedFrame represents a rendered frame from the terminal
type EmittedFrame struct {
	Rows []string // Each row is a string with ANSI escape codes embedded
}

// New creates a new headless terminal emulator
func New(cols, rows int) (*Emulator, error) {
	e := &Emulator{
		mainScreen:  newScreen(cols, rows),
		altScreen:   newScreen(cols, rows),
		frameRate:   time.Second / 30, // Default 30 FPS
		stopChan:    make(chan struct{}),
		viewFlags:   make([]bool, viewFlagCount),
		viewInts:    make([]int, viewIntCount),
		viewStrings: make([]string, viewStringCount),
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
	err := pty.Setsize(e.pty, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
		X:    uint16(cols * 8),
		Y:    uint16(rows * 16),
	})
	if err != nil {
		return err
	}

	e.mainScreen.setSize(cols, rows)
	e.altScreen.setSize(cols, rows)

	return nil
}

// SetFrameRate sets the internal render loop framerate
func (e *Emulator) SetFrameRate(fps int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.frameRate = time.Second / time.Duration(fps)
}

// GetScreen returns the current rendered screen as ANSI strings
func (e *Emulator) GetScreen() EmittedFrame {
	e.mu.RLock()
	defer e.mu.RUnlock()

	screen := e.currentScreen()
	rows := make([]string, screen.size.Y)
	
	for y := 0; y < screen.size.Y; y++ {
		rows[y] = screen.renderLineANSI(y)
	}

	return EmittedFrame{Rows: rows}
}

// FeedInput processes raw ANSI input (typically from PTY)
func (e *Emulator) FeedInput(data []byte) {
	// This will be called by the PTY read loop
	// For now, we don't need to expose this publicly since PTY handles it
}

// StartCommand starts a command in the terminal
func (e *Emulator) StartCommand(cmd *exec.Cmd) error {
	e.mu.Lock()
	defer e.mu.Unlock()

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

	return cmd.Start()
}

// Write sends data to the PTY (keyboard input)
func (e *Emulator) Write(data []byte) (int, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
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

// Close shuts down the emulator
func (e *Emulator) Close() error {
	close(e.stopChan)
	
	if e.tty != nil {
		e.tty.Close()
	}
	if e.pty != nil {
		e.pty.Close()
	}
	
	return nil
}

// currentScreen returns the currently active screen (main or alt)
func (e *Emulator) currentScreen() *screen {
	if e.onAltScreen {
		return e.altScreen
	}
	return e.mainScreen
}

// switchScreen toggles between main and alternate screen
func (e *Emulator) switchScreen() {
	e.onAltScreen = !e.onAltScreen
}