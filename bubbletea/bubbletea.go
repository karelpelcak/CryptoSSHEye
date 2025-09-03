package bubbletea

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/guptarohit/asciigraph"
	"github.com/karelpelcak/CryptoSSHEye/utils"
)

var (
	green = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
	red   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	white = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
)

const (
	maxPoints = 1800
	usdtToCzk = 21
)

type PriceMsg struct {
	V float64
}
type StatusMsg string

type keymap struct {
	Quit key.Binding
	Help key.Binding
}

func newKeymap() keymap {
	return keymap{
		Quit: key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Help: key.NewBinding(key.WithKeys("?")),
	}
}

type model struct {
	prices []float64
	last   float64
	mu     sync.Mutex

	vp   viewport.Model
	help help.Model
	km   keymap

	priceCh  <-chan float64
	logf     func(string, ...any)
	showHelp bool
}

func InitialModel(width, height int, ch <-chan float64, logf func(string, ...any)) model {
	vp := viewport.New(width, height)
	vp.MouseWheelEnabled = false
	return model{
		vp:      vp,
		help:    help.New(),
		km:      newKeymap(),
		priceCh: ch,
		logf:    logf,
	}
}

func (k keymap) ShortHelp() []key.Binding { return []key.Binding{k.Quit, k.Help} }
func (k keymap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Quit, k.Help}}
}

func (m model) Init() tea.Cmd {
	return m.awaitPrice()
}

func (m *model) awaitPrice() tea.Cmd {
	return func() tea.Msg {
		v, ok := <-m.priceCh
		if !ok {
			return StatusMsg("stream closed")
		}
		return PriceMsg{V: v}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.vp.Width = msg.Width
		m.vp.Height = msg.Height
		return m, nil

	case PriceMsg:
		m.mu.Lock()
		m.last = msg.V
		m.prices = append(m.prices, msg.V)
		if len(m.prices) > maxPoints {
			m.prices = m.prices[len(m.prices)-maxPoints:]
		}
		m.mu.Unlock()
		m.render()
		return m, m.awaitPrice()

	case StatusMsg:
		m.vp.SetContent(fmt.Sprintf("%s\n\nPress q to quit.", string(msg)))
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.km.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.km.Help):
			m.showHelp = !m.showHelp
			m.render()
		}
	}
	return m, nil
}

func (m *model) render() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.prices) == 0 {
		m.vp.SetContent("Connecting to Binance…")
		return
	}

	trendColor := white
	if len(m.prices) > 1 {
		last := m.prices[len(m.prices)-2]
		if m.last > last {
			trendColor = green
		} else if m.last < last {
			trendColor = red
		}
	}

	w := max(20, m.vp.Width-4)
	h := max(8, min(20, m.vp.Height-6))
	graph := asciigraph.Plot(m.prices,
		asciigraph.Width(w),
		asciigraph.Height(h),
		asciigraph.Caption(trendColor.Render(fmt.Sprintf("BTC/USDT %.2f", m.last))),
	)

	minV, maxV := stats(m.prices)
	span := maxV - minV
	pct := 0.0
	if len(m.prices) > 1 && m.prices[0] != 0 {
		pct = (m.prices[len(m.prices)-1] - m.prices[0]) / m.prices[0] * 100
	}

	infoGraph := fmt.Sprintf("Δ: %.2f (%.2f%%)  %s",
		span, pct, time.Now().Format(time.Kitchen))

	infoUsdt := fmt.Sprintf("Last: %susdt  Min: %susdt  Max: %susdt",
		utils.FormatFloatWithSpaces(m.last),
		utils.FormatFloatWithSpaces(minV),
		utils.FormatFloatWithSpaces(maxV),
	)

	infoCzk := fmt.Sprintf("Last: %sczk  Min: %sczk  Max: %sczk",
		utils.FormatFloatWithSpaces(m.last*usdtToCzk),
		utils.FormatFloatWithSpaces(minV*usdtToCzk),
		utils.FormatFloatWithSpaces(maxV*usdtToCzk),
	)

	content := "BTC/USDT Live Price"
	content += "\n\n\n" + graph + "\n\n" + infoGraph
	content += "\n" + infoUsdt
	content += "\n" + infoCzk
	content += "\n\n" + m.help.View(m.km)
	m.vp.SetContent(content)
}

func (m model) View() string { return m.vp.View() }

func stats(xs []float64) (float64, float64) {
	if len(xs) == 0 {
		return 0, 0
	}
	minV, maxV := math.MaxFloat64, -math.MaxFloat64
	for _, v := range xs {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	return minV, maxV
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
