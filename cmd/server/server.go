package main

import (
	"context"
	"crypto/tls"
	"flag"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/hashicorp/yamux"
)

func main() {
	controlAddr := flag.String(
		"tunnel",
		":10100",
		"Address to accept tunnel connections from host",
	)
	localAddr := flag.String(
		"listen",
		":3000",
		"Address to accept incoming public connections",
	)
	useTLS := flag.Bool(
		"tls",
		false,
		"enable TLS",
	)
	cert := flag.String(
		"cert",
		"",
		"tls cert file",
	)
	key := flag.String(
		"key",
		"",
		"tls key file",
	)
	flag.Parse()

	srv := &Server{
		controlAddr: *controlAddr,
		localAddr:   *localAddr,
		useTLS:      *useTLS,
		cert:        *cert,
		key:         *key,
	}

	if err := srv.Run(); err != nil {
		log.Fatal(err)
	}
}

type Server struct {
	controlAddr string
	localAddr   string

	useTLS bool
	cert   string
	key    string

	mu       sync.RWMutex
	session  *yamux.Session
	shutting bool
}

func (s *Server) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Перехват SIGINT/SIGTERM
	go s.handleSignals(cancel)

	controlLn, err := s.createControlListener()
	if err != nil {
		return err
	}
	defer controlLn.Close()

	publicLn, err := net.Listen("tcp", s.localAddr)
	if err != nil {
		return err
	}
	defer publicLn.Close()

	log.Println("[Server] Started")

	// Ожидание подключения хоста
	go s.acceptControl(ctx, controlLn)
	// Ожидание подключения клиентов
	go s.acceptPublic(ctx, publicLn)
	// Блокирование исполнения до сигнала
	<-ctx.Done()

	// Закрытие сессии
	s.mu.Lock()
	s.shutting = true
	if s.session != nil {
		s.session.Close()
	}
	s.mu.Unlock()

	log.Println("[Server] Stopped")
	return nil
}

// Открытие TCP или TLS listener в зависимости от конфига
func (s *Server) createControlListener() (net.Listener, error) {
	if !s.useTLS {
		return net.Listen("tcp", s.controlAddr)
	}

	cert, err := tls.LoadX509KeyPair(s.cert, s.key)
	if err != nil {
		return nil, err
	}

	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}

	return tls.Listen("tcp", s.controlAddr, cfg)
}

// Принимает подключения хоста
// одновременно активна одна сессия
func (s *Server) acceptControl(ctx context.Context, ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if s.isShutting() {
				return
			}
			continue
		}

		go s.handleControl(conn)
	}
}

// Инициализация yamux-сессии и ожидание её закрытия
func (s *Server) handleControl(conn net.Conn) {
	// TCP keep-alive и отключение буферизации мелких пакетов
	if tcp, ok := conn.(*net.TCPConn); ok {
		tcp.SetKeepAlive(true)
		tcp.SetKeepAlivePeriod(30 * time.Second)
		tcp.SetNoDelay(true)
	}

	cfg := yamux.DefaultConfig()
	cfg.EnableKeepAlive = true
	cfg.KeepAliveInterval = 10 * time.Second

	session, err := yamux.Server(conn, cfg)
	if err != nil {
		conn.Close()
		return
	}

	// Замена старой сессии новой (хост переподключился)
	s.mu.Lock()
	if s.session != nil {
		s.session.Close()
	}
	s.session = session
	s.mu.Unlock()

	log.Println("[Server] Control session established")

	<-session.CloseChan()

	s.mu.Lock()
	if s.session == session {
		s.session = nil
	}
	s.mu.Unlock()

	log.Println("[Server] control session closed")
}

// Приём входящие TCP-соединения от внешних клиентов
func (s *Server) acceptPublic(ctx context.Context, ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if s.isShutting() {
				return
			}
			continue
		}

		go s.handlePublic(conn)
	}
}

// Открытие стрим к хосту и проксирование в него внешнего клиента
func (s *Server) handlePublic(clientConn net.Conn) {
	if tcp, ok := clientConn.(*net.TCPConn); ok {
		tcp.SetNoDelay(true)
	}

	session := s.getSession()
	if session == nil {
		// Хост не подключён: закрытие соединения
		clientConn.Close()
		return
	}

	// Новый логический стрим внутри туннеля к хосту
	stream, err := session.Open()
	if err != nil {
		clientConn.Close()
		return
	}

	// Отправка данных от клиента к хосту
	go func() {
		defer stream.Close()
		defer clientConn.Close()
		io.Copy(stream, clientConn)
	}()

	// Отправка данных от хосту к клиенту
	go func() {
		defer stream.Close()
		defer clientConn.Close()
		io.Copy(clientConn, stream)
	}()
}

func (s *Server) getSession() *yamux.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.session
}

func (s *Server) isShutting() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.shutting
}

// Отмена контекста при получении SIGINT или SIGTERM
func (s *Server) handleSignals(cancel context.CancelFunc) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	cancel()
}
