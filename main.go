package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"sync/atomic"
	"time"
)

const (
	DefaultPort          = 2222
	DefaultDelay         = 10000
	DefaultMaxLineLength = 32
	DefaultMaxClients    = 4096
)

var (
	currentClients int64
	totalConnects  int64
	bytesSent      int64
)

type Config struct {
	Port          int
	Delay         time.Duration
	MaxLineLength int
	MaxClients    int64
	BindFamily    string
}

func main() {
	port := flag.Int("p", DefaultPort, "Listening port")
	delayMs := flag.Int("d", DefaultDelay, "Message millisecond delay")
	maxLineLen := flag.Int("l", DefaultMaxLineLength, "Maximum banner line length (3-255)")
	maxClients := flag.Int64("m", DefaultMaxClients, "Maximum number of clients")
	useV4 := flag.Bool("4", false, "Bind to IPv4 only")
	useV6 := flag.Bool("6", false, "Bind to IPv6 only")
	help := flag.Bool("h", false, "Print this help message")
	flag.Parse()

	if *help {
		flag.Usage()
		os.Exit(0)
	}

	network := "tcp"
	if *useV4 {
		network = "tcp4"
	} else if *useV6 {
		network = "tcp6"
	}

	config := Config{
		Port:          *port,
		Delay:         time.Duration(*delayMs) * time.Millisecond,
		MaxLineLength: *maxLineLen,
		MaxClients:    *maxClients,
		BindFamily:    network,
	}

	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.LUTC)

	go statsReporter()

	listenAddr := fmt.Sprintf(":%d", config.Port)
	listener, err := net.Listen(config.BindFamily, listenAddr)
	if err != nil {
		log.Fatalf("Fatal: %v", err)
	}
	defer listener.Close()

	log.Printf("OREXIS listening on %s %s", config.BindFamily, listenAddr)
	log.Printf("Config: Delay=%v, MaxLineLength=%d, MaxClients=%d", config.Delay, config.MaxLineLength, config.MaxClients)

	// Main loop
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}

		if atomic.LoadInt64(&currentClients) >= config.MaxClients {
			conn.Close()
			continue
		}

		go handleClient(conn, config)
	}
}

func statsReporter() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		curr := atomic.LoadInt64(&currentClients)
		total := atomic.LoadInt64(&totalConnects)
		bytes := atomic.LoadInt64(&bytesSent)
		
		log.Printf("STATS: CurrentClients=%d TotalConnects=%d TotalBytesSent=%d", curr, total, bytes)
	}
}

func handleClient(conn net.Conn, config Config) {
	atomic.AddInt64(&currentClients, 1)
	atomic.AddInt64(&totalConnects, 1)

	defer func() {
		conn.Close()
		atomic.AddInt64(&currentClients, -1)

		log.Printf("DISCONNECT host=%s", conn.RemoteAddr().String())
	}()

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		// 受信バッファを最小に
		if err := tcpConn.SetReadBuffer(1); err != nil {
			log.Printf("SetReadBuffer error: %v", err)
		}
	}

	host, port, _ := net.SplitHostPort(conn.RemoteAddr().String())
	log.Printf("ACCEPT host=%s port=%s clients=%d", host, port, atomic.LoadInt64(&currentClients))

	writer := bufio.NewWriter(conn)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for {
		line := generateLine(rng, config.MaxLineLength)

		n, err := writer.WriteString(line)
		if err != nil {
			// クライアントが切断した場合など
			return
		}
		if err := writer.Flush(); err != nil {
			return
		}

		atomic.AddInt64(&bytesSent, int64(n))
		time.Sleep(config.Delay)
	}
}

func generateLine(rng *rand.Rand, maxLen int) string {
	length := 3 + rng.Intn(maxLen-2)
	
	line := make([]byte, length)
	for i := 0; i < length-2; i++ {
		// ASCII 32(Space) から 126(~) の範囲の文字
		line[i] = byte(32 + rng.Intn(95))
	}
	// CR LF
	line[length-2] = 13
	line[length-1] = 10

	// もし偶然 "SSH-" で始まってしまったら、プロトコルエラーで即切断されるのを防ぐため書き換える
	if length >= 4 && string(line[:4]) == "SSH-" {
		line[0] = 'X'
	}

	return string(line)
}