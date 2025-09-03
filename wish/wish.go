package wish

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	ssh "github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	wtea "github.com/charmbracelet/wish/bubbletea"
	"github.com/karelpelcak/CryptoSSHEye/bubbletea"
	"github.com/karelpelcak/CryptoSSHEye/utils"
	"github.com/karelpelcak/CryptoSSHEye/websocket"
)

func InitWishServer() {
	addr := ":23234"
	hostKeyPath := utils.EnvOr("SSH_HOST_KEY", "ssh_host_ed25519")

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

				go websocket.WsReader(ctx, priceCh, func(f string, a ...any) { logger.Printf("[ws] "+f, a...) })

				go func() {
					<-s.Context().Done()
					cancel()
				}()

				m := bubbletea.InitialModel(width, height, priceCh, func(f string, a ...any) { logger.Printf("[model] "+f, a...) })
				return m, []tea.ProgramOption{}
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
