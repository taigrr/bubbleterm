package emulator

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestInvalidDimensions(t *testing.T) {
	sizes := []struct {
		name       string
		cols, rows int
	}{
		{"zero columns", 0, 2},
		{"zero rows", 2, 0},
		{"negative columns", -1, 2},
		{"negative rows", 2, -1},
	}
	constructors := []struct {
		name string
		new  func(int, int) (*Emulator, error)
	}{
		{"PTY", New},
		{"pipes", func(cols, rows int) (*Emulator, error) {
			return NewFromPipes(cols, rows, strings.NewReader(""), nil)
		}},
	}
	for _, constructor := range constructors {
		for _, size := range sizes {
			t.Run(constructor.name+"/"+size.name, func(t *testing.T) {
				defer func() {
					if recovered := recover(); recovered != nil {
						t.Errorf("constructor panicked: %v", recovered)
					}
				}()
				emu, err := constructor.new(size.cols, size.rows)
				if emu != nil {
					defer emu.Close()
					t.Error("invalid dimensions returned an emulator")
				}
				if !errors.Is(err, ErrInvalidSize) {
					t.Errorf("error = %v, want ErrInvalidSize", err)
				}
			})
		}
	}
	for _, size := range sizes {
		t.Run("resize/"+size.name, func(t *testing.T) {
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			emu, err := NewFromPipes(4, 2, reader, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer emu.Close()
			before := emu.GetScreen()
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Errorf("Resize panicked: %v", recovered)
				}
			}()
			if err := emu.Resize(size.cols, size.rows); !errors.Is(err, ErrInvalidSize) {
				t.Errorf("Resize error = %v, want ErrInvalidSize", err)
			}
			if emu.width != 4 || emu.height != 2 || emu.vt.Width() != 4 || emu.vt.Height() != 2 {
				t.Fatal("invalid resize changed terminal dimensions")
			}
			after := emu.GetScreen()
			if strings.Join(before.Rows, "\n") != strings.Join(after.Rows, "\n") || len(after.Damage) != 0 {
				t.Fatal("invalid resize changed screen or damage state")
			}
		})
	}
}
