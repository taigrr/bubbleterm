package emulator

import (
	"strings"
	"testing"
)

func TestScreenDamageTracking(t *testing.T) {
	s := newScreen(5, 3)

	// Initial screen should report full damage.
	initialDamage := s.consumeDamage()
	if len(initialDamage) != 3 {
		t.Fatalf("expected 3 damaged rows, got %d", len(initialDamage))
	}
	for idx, d := range initialDamage {
		if d.Row != idx || d.X1 != 0 || d.X2 != 5 {
			t.Fatalf("unexpected damage entry %#v", d)
		}
	}

	// No more damage after consumption.
	if len(s.consumeDamage()) != 0 {
		t.Fatalf("expected no damage after consumption")
	}

	// Writing characters should mark the affected segment.
	s.setCursorPos(1, 1)
	s.writeRunes([]rune("hi"))
	damage := s.consumeDamage()
	if len(damage) != 1 {
		t.Fatalf("expected single damaged row, got %d", len(damage))
	}
	d := damage[0]
	if d.Row != 1 || d.X1 != 1 || d.X2 != 3 {
		t.Fatalf("unexpected damage data %+v", d)
	}
}

func TestEmulatorGetScreenDamage(t *testing.T) {
	e := &Emulator{
		mainScreen:  newScreen(4, 2),
		altScreen:   newScreen(4, 2),
		viewFlags:   make([]bool, viewFlagCount),
		viewInts:    make([]int, viewIntCount),
		viewStrings: make([]string, viewStringCount),
	}

	frame := e.GetScreen()
	if len(frame.Damage) != 2 {
		t.Fatalf("expected initial full damage, got %d entries", len(frame.Damage))
	}

	// Consume damage; next frame without writes should have no damage.
	frame = e.GetScreen()
	if len(frame.Damage) != 0 {
		t.Fatalf("expected no damage without writes, got %d", len(frame.Damage))
	}

	// Write a character and ensure damage is reported.
	e.mu.Lock()
	e.currentScreen().setCursorPos(0, 0)
	e.currentScreen().writeRunes([]rune("z"))
	e.mu.Unlock()

	frame = e.GetScreen()
	if len(frame.Damage) != 1 {
		t.Fatalf("expected damage after write, got %d", len(frame.Damage))
	}
	if frame.Damage[0].Row != 0 {
		t.Fatalf("expected damage on row 0, got %d", frame.Damage[0].Row)
	}
	if len(frame.Rows) != 2 {
		t.Fatalf("expected full row buffer, got %d rows", len(frame.Rows))
	}
}

func TestLineFeedScrollStaysOnBottomRow(t *testing.T) {
	s := newScreen(5, 3)
	// Consume initial damage.
	s.consumeDamage()

	writeLine := func(text string) {
		s.writeRunes([]rune(text))
		s.moveCursor(-s.cursorPos.X, 1, true, true)
	}

	writeLine("0")
	writeLine("1")

	// This line feed should trigger a scroll and leave cursor on the last row.
	writeLine("2")
	if s.cursorPos.Y != s.bottomMargin {
		t.Fatalf("expected cursor on bottom row after scroll, got %d", s.cursorPos.Y)
	}

	// Next line should be written to the bottom row.
	s.writeRunes([]rune("3"))

	got := []string{
		strings.TrimRight(s.lineString(0), " "),
		strings.TrimRight(s.lineString(1), " "),
		strings.TrimRight(s.lineString(2), " "),
	}
	want := []string{"1", "2", "3"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d = %q, want %q", i, got[i], want[i])
		}
	}
}
