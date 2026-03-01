package main

import (
	"crypto/tls"
	"flag"
	"io"
	"log"
	"net"
	"time"

	"github.com/hashicorp/yamux"
)

func main() {
	serverAddr := flag.String(
		"server",
		"127.0.0.1:10100",
		"Tunnel server address to connect to",
	)
	forwardAddr := flag.String(
		"forward",
		"127.0.0.1:8080",
		"Local service address to forward traffic to",
	)
	useTLS := flag.Bool(
		"tls",
		false,
		"Connect to the server via TLS",
	)
	tlsSkipVerify := flag.Bool(
		"tls-skip-verify",
		false,
		"Do not verify server certificate (for self-signed)",
	)
	flag.Parse()

	h := &Host{
		forwardAddr:   *forwardAddr,
		serverAddr:    *serverAddr,
		useTLS:        *useTLS,
		tlsSkipVerify: *tlsSkipVerify,
	}

	log.Printf("[TLS] Use: %v\n", h.useTLS)
	log.Printf("[TLS] Skip verify: %v\n", h.tlsSkipVerify)
	log.Printf("[Host] Listen: %v\n", h.serverAddr)
	log.Printf("[Host] Proxy: %v\n", h.forwardAddr)

	h.runForever()
}

type Host struct {
	forwardAddr   string
	serverAddr    string
	useTLS        bool
	tlsSkipVerify bool
}

// Установка TCP или TLS соединение с сервером
func (h *Host) dialControl() (net.Conn, error) {
	if !h.useTLS {
		return net.Dial("tcp", h.serverAddr)
	}

	config := &tls.Config{
		InsecureSkipVerify: h.tlsSkipVerify,
		MinVersion:         tls.VersionTLS13,
	}

	return tls.Dial("tcp", h.serverAddr, config)
}

// Попытка бесконечного переподключения к серверу
func (h *Host) runForever() {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		err := h.runSession()
		log.Printf("[Host] Session ended: %v", err)

		time.Sleep(backoff)

		// Удовоение задержки, до backoff
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// Установка yamux-сессии и обслуживание до разрыва
func (h *Host) runSession() error {
	conn, err := h.dialControl()
	if err != nil {
		return err
	}
	defer conn.Close()

	cfg := yamux.DefaultConfig()

	// Настройка клиентской части yamux
	session, err := yamux.Client(conn, cfg)
	if err != nil {
		return err
	}
	defer session.Close()

	// Обработка клиентов
	// stream - один внешний клиент на сервере
	for {
		stream, err := session.Accept()
		if err != nil {
			return err
		}

		go h.handleStream(stream)
	}
}

// Подключение к локальному сервису и проксирование стрима в обе стороны
func (h *Host) handleStream(stream net.Conn) {
	defer stream.Close()

	backend, err := net.Dial("tcp", h.forwardAddr)
	if err != nil {
		return
	}
	defer backend.Close()

	// Проксирование стрима: сервер -> локальный сервис
	go io.Copy(backend, stream)
	// Проксирование стрима: локальный сервис -> сервер
	io.Copy(stream, backend)
}
