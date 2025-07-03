package emulator

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/google/uuid"
)

// Emulator is a headless terminal emulator that maintains internal state
// and renders to a framebuffer instead of directly to screen
type Emulator struct {
	mu sync.RWMutex
	id string

	// Terminal state
	mainScreen  *screen
	altScreen   *screen
	onAltScreen bool

	// PTY for process communication
	pty, tty *os.File

	// Process tracking
	cmd           *exec.Cmd
	processExited bool
	onExit        func(string) // Callback when process exits, receives emulator ID

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
		id:          uuid.New().String(), // Generate a unique ID
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
	// Debug: print resize info
	// fmt.Printf("Resizing PTY to %dx%d\n", cols, rows)

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
	err := e.cmd.Wait()

	e.mu.Lock()
	e.processExited = true
	onExit := e.onExit
	id := e.id
	e.mu.Unlock()

	// Call the exit callback if set
	if onExit != nil {
		onExit(id)
	}

	// Log the exit for debugging
	if err != nil {
		fmt.Printf("Process %s exited with error: %v\n", id, err)
	} else {
		fmt.Printf("Process %s exited successfully\n", id)
	}
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

// SendMouse sends a mouse event to the terminal in SGR format
func (e *Emulator) SendMouse(button int, x, y int, pressed bool) error {
	e.mu.RLock()
	mouseMode := e.viewInts[VIMouseMode]
	mouseEncoding := e.viewInts[VIMouseEncoding]
	e.mu.RUnlock()

	// For motion events (button == -1), we need special handling
	isMotion := button == -1

	// If mouse mode is disabled, enable basic mouse mode for compatibility
	if mouseMode == MMNone {
		e.mu.Lock()
		e.viewInts[VIMouseMode] = MMPressReleaseMoveAll // Enable all mouse events
		e.viewInts[VIMouseEncoding] = MESGR             // Use SGR encoding
		mouseMode = MMPressReleaseMoveAll
		mouseEncoding = MESGR
		e.mu.Unlock()
	}

	// Check if this event should be sent based on mouse mode
	switch mouseMode {
	case MMPress:
		if !pressed || isMotion {
			return nil // Only send press events
		}
	case MMPressRelease:
		if isMotion {
			return nil // No motion events
		}
	case MMPressReleaseMove:
		// Send press, release, and motion (when button is pressed)
		// For motion, we'll use button 0 to indicate motion
		if isMotion {
			button = 32 // Motion event code
		}
	case MMPressReleaseMoveAll:
		// Send all events including motion without buttons
		if isMotion {
			button = 35 // Motion without button code
		}
	}

	// Format mouse event based on encoding
	var mouseSeq string
	switch mouseEncoding {
	case MESGR:
		// SGR format: \x1b[<button;x;y;M/m
		action := "M"
		if !pressed && !isMotion {
			action = "m"
		}
		mouseSeq = fmt.Sprintf("\x1b[<%d;%d;%d%s", button, x+1, y+1, action)
	case MEUTF8:
		// UTF-8 format: \x1b[M + 3 bytes
		buttonCode := 32 + button
		if !pressed && !isMotion {
			buttonCode += 3
		}
		mouseSeq = fmt.Sprintf("\x1b[M%c%c%c",
			buttonCode,
			32+x+1,
			32+y+1)
	default: // MEX10
		// X10 format: \x1b[M + 3 bytes
		if x > 222 || y > 222 {
			return nil // X10 can't handle large coordinates
		}
		mouseSeq = fmt.Sprintf("\x1b[M%c%c%c",
			32+button,
			32+x+1,
			32+y+1)
	}

	_, err := e.Write([]byte(mouseSeq))
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
