// main.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	ssh "github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	wtea "github.com/charmbracelet/wish/bubbletea"
	"github.com/gorilla/websocket"
	asciigraph "github.com/guptarohit/asciigraph"
)

const (
	binanceWS = "wss://stream.binance.com:9443/ws/btcusdt@miniTicker"
	maxPoints = 1800
)

var (
	green = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
	red   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	white = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
)

type PriceMsg struct{ V float64 }
type StatusMsg string

func wsReader(ctx context.Context, out chan<- float64, logf func(string, ...any)) {
	defer close(out)
	dialer := websocket.Dialer{}
	retry := 0

	for {
		c, _, err := dialer.DialContext(ctx, binanceWS, nil)
		if err != nil {
			retry++
			backoff := time.Duration(min(30, 1<<min(6, retry))) * time.Second
			logf("WS connect failed: %v — retrying in %s", err, backoff)
			select {
			case <-time.After(backoff):
				continue
			case <-ctx.Done():
				return
			}
		}
		retry = 0
		logf("Connected to Binance stream")

		c.SetReadLimit(1 << 20)
		_ = c.SetReadDeadline(time.Now().Add(90 * time.Second))
		c.SetPongHandler(func(string) error {
			_ = c.SetReadDeadline(time.Now().Add(90 * time.Second))
			return nil
		})

		go func() {
			ticker := time.NewTicker(45 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_ = c.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second))
				}
			}
		}()

		for {
			_, data, err := c.ReadMessage()
			if err != nil {
				logf("WS read error: %v", err)
				_ = c.Close()
				break
			}

			var tm struct {
				C string `json:"c"` // current price
			}
			if err := json.Unmarshal(data, &tm); err != nil {
				continue
			}
			v, err := strconv.ParseFloat(tm.C, 64)
			if err != nil {
				continue
			}

			logf("Price received: %.2f", v)

			select {
			case out <- v:
			default: // drop if channel full
			case <-ctx.Done():
				_ = c.Close()
				return
			}
		}

		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

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

func (k keymap) ShortHelp() []key.Binding { return []key.Binding{k.Quit, k.Help} }
func (k keymap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Quit, k.Help}}
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

func initialModel(width, height int, ch <-chan float64, logf func(string, ...any)) model {
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

func (m model) Init() tea.Cmd { return m.awaitPrice() }

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

	info := fmt.Sprintf("Last: %.2f  Min: %.2f  Max: %.2f  Δ: %.2f (%.2f%%)  %s",
		m.last, minV, maxV, span, pct, time.Now().Format(time.Kitchen))

	content := "BTC/USDT Live Price"
	content += "\n\n\n" + graph + "\n\n" + info
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

func main() {
	addr := ":23234"
	hostKeyPath := envOr("SSH_HOST_KEY", "ssh_host_ed25519")

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("cannot listen on %s: %v", addr, err)
	}
	_ = ln.Close()

	logger := log.New(os.Stdout, "[btc-ssh] ", log.LstdFlags)

	server, err := wish.NewServer(
		wish.WithAddress(addr),
		wish.WithHostKeyPath(hostKeyPath),
		wish.WithMiddleware(
			wtea.Middleware(func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
				pty, _, ok := s.Pty()
				width, height := 80, 24
				if ok {
					if pty.Window.Width > 0 {
						width = pty.Window.Width
					}
					if pty.Window.Height > 0 {
						height = pty.Window.Height
					}
				}

				ctx, cancel := context.WithCancel(context.Background())
				priceCh := make(chan float64, 256)

				go wsReader(ctx, priceCh, func(f string, a ...any) { logger.Printf("[ws] "+f, a...) })

				go func() {
					<-s.Context().Done()
					cancel()
				}()

				m := initialModel(width, height, priceCh, func(f string, a ...any) { logger.Printf("[model] "+f, a...) })
				return m, []tea.ProgramOption{} // Bez AltScreen pro jednoduché testování
			}),
		),
	)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		logger.Println("shutting down…")
		_ = server.Close()
	}()

	logger.Printf("listening on %s (ssh). Connect with: ssh -t -p %s <user>@<host>", addr, addr[1:])
	if err := server.ListenAndServe(); err != nil && err != ssh.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
