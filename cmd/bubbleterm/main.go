package main

import (
	"log"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/taigrr/bib/bubbleterm"
)

type model struct {
	terminal *bubbleterm.Model
	err      error
}

func main() {
	// Create a new terminal bubble and start htop
	cmd := exec.Command("htop")
	terminal, err := bubbleterm.NewWithCommand(80, 24, "default", cmd)
	if err != nil {
		log.Fatal(err)
	}

	m := model{
		terminal: terminal,
	}

	p := tea.NewProgram(&m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

func (m *model) Init() tea.Cmd {
	return m.terminal.Init()
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.terminal.Close()
			return m, tea.Quit
		}
	}

	// Forward all messages to the terminal bubble
	var cmd tea.Cmd
	terminalModel, cmd := m.terminal.Update(msg)
	m.terminal = terminalModel.(*bubbleterm.Model)

	return m, cmd
}

func (m *model) View() string {
	if m.err != nil {
		return "Error: " + m.err.Error()
	}

	return m.terminal.View()
}

