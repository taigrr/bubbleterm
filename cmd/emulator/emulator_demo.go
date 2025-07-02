package main

import (
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/taigrr/bib/emulator"
)

func main() {
	// Create a new emulator
	emu, err := emulator.New(80, 24)
	if err != nil {
		log.Fatal(err)
	}
	defer emu.Close()

	// Start a simple command
	cmd := exec.Command("htop")
	err = emu.StartCommand(cmd)
	if err != nil {
		log.Fatal(err)
	}

	// Wait a moment for output
	time.Sleep(10 * time.Second)

	// Get the screen
	frame := emu.GetScreen()

	fmt.Println("Terminal output:")
	for i, row := range frame.Rows {
		fmt.Printf("%2d: %s\n", i, row)
	}
	fmt.Println("resizing!")

	emu.Resize(100, 40)
	// Wait a moment for output after resizing
	time.Sleep(1 * time.Second)
	fmt.Println("Terminal output after resizing:")
	frame = emu.GetScreen()
	for i, row := range frame.Rows {
		fmt.Printf("%2d: %s\n", i, row)
	}
}
