package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.design/x/clipboard"

	pb "github.com/Roarcannotprogramming/clipipe/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	addr     = flag.String("addr", "127.0.0.1", "The server address")
	port     = flag.Int("port", 8799, "The server port")
	hostname = flag.String("hostname", "", "The hostname")
)

var noNotify = make(chan struct{}, 10)

// Prepare for muli-thread
type clipContent struct {
	Content []byte
	ctx     context.Context
	mu      sync.Mutex
}

func (cc *clipContent) Init() error {
	err := clipboard.Init()
	if err != nil {
		return err
	}
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.ctx = context.Background()
	return nil
}

func (cc *clipContent) GetClipboard() ([]byte, error) {
	b := clipboard.Read(clipboard.FmtText)
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.Content = b
	return b, nil
}

// Block
func (cc *clipContent) WatchClipboard(data chan []byte) {
	if cc.ctx == nil {
		panic("cc.ctx is nil")
	}
	new_data := clipboard.Watch(cc.ctx, clipboard.FmtText)
	for {
		d := <-new_data
		cc.mu.Lock()
		cc.Content = d
		cc.mu.Unlock()
		log.Printf("Get new clipboard data")
		select {
		case <-noNotify:
			log.Printf("No notification")
			continue
		default:
			data <- d
		}
	}
}

func (cc *clipContent) WriteClipboard() {
	clipboard.Write(clipboard.FmtText, cc.Content)
}

type ClipClient struct {
	pb     *pb.ClipClient
	cc     *clipContent
	pb_ctx context.Context
}

func NewClipClient(pb *pb.ClipClient) (*ClipClient, error) {
	cc := &clipContent{}
	err := cc.Init()
	if err != nil {
		return nil, err
	}
	if pb == nil {
		return nil, fmt.Errorf("pb is nil")
	}
	return &ClipClient{pb: pb, cc: cc, pb_ctx: context.Background()}, nil
}

func GetHostname() (string, error) {
	if *hostname != "" {
		return *hostname, nil
	}
	os_hostname, err := os.Hostname()
	hostname = &os_hostname
	if err != nil {
		return "", err
	}
	return *hostname, nil
}

func (cclient *ClipClient) KeepAlive() {
	for {
		hostname, err := GetHostname()
		if err != nil {
			log.Fatalf("failed to get hostname: %v", err)
		}
		status := pb.Status{Code: pb.Code_OK, Message: "ping"}
		ping_ctx, cancel := context.WithTimeout(cclient.pb_ctx, time.Second)
		_, ping_err := (*cclient.pb).Ping(ping_ctx, &pb.PingRequest{Id: hostname, Status: &status})
		if ping_err != nil {
			log.Fatalf("failed to ping: %v", ping_err)
		}
		cancel()
		// log.Printf("Ping response: %s, message: %s", r.Id, r.Status.Message)
		time.Sleep(5 * time.Second)
	}
}

func (cclient *ClipClient) WatchClipboardSend() {
	data := make(chan []byte)
	go cclient.cc.WatchClipboard(data)
	hostname, err := GetHostname()
	if err != nil {
		log.Fatalf("failed to get hostname: %v", err)
	}
	for {
		d := <-data
		log.Printf("[*] To Push: %s", d)

		pushRequest := &pb.PushRequest{
			Id: hostname,
			Status: &pb.Status{
				Code:    pb.Code_OK,
				Message: "push",
			},
			Msg: d,
		}
		push_ctx, cancel := context.WithTimeout(cclient.pb_ctx, time.Second)
		r, push_err := (*cclient.pb).Push(push_ctx, pushRequest)
		if push_err != nil {
			log.Fatalf("failed to push: %v", push_err)
		}
		cancel()
		if r.Id != hostname || r.Status.Code != pb.Code_OK {
			log.Fatalf("failed to push: %v", push_err)
		}
	}
}

func (cclient *ClipClient) ConnectRecvUpdate() {
	hostname, err := GetHostname()
	if err != nil {
		log.Fatalf("failed to get hostname: %v", err)
	}
	connrequest := &pb.ConnRequest{
		Id: hostname,
		Status: &pb.Status{
			Code:    pb.Code_OK,
			Message: "connect",
		},
	}
	// stream_ctx, cancel := context.WithTimeout(cclient.pb_ctx, 10*time.Second)
	stream, err := (*cclient.pb).GetStream(cclient.pb_ctx, connrequest)
	if err != nil {
		log.Fatalf("failed to get stream: %v", err)
	}
	log.Printf("Get stream success")
	// defer cancel()
	for {
		log.Printf("Waiting for update")
		update, err := stream.Recv()
		log.Printf("Get update")
		if err != nil {
			log.Fatalf("failed to recv: %v", err)
		}
		if update.Status.Code != pb.Code_OK {
			log.Fatalf("failed to recv content: %v", err)
		}
		log.Printf("[*] Receive : %s", update.Msg)
		if update.Id != hostname {
			cclient.cc.mu.Lock()
			if !bytes.Equal(cclient.cc.Content, update.Msg) {
				cclient.cc.Content = update.Msg
			} else {
				cclient.cc.mu.Unlock()
				continue
			}
			cclient.cc.mu.Unlock()
			go func() {
				noNotify <- struct{}{}
				log.Printf("[+] Update clipboard from %s: %s", update.Id, update.Msg)
				cclient.cc.WriteClipboard()
			}()
		}
	}
}

func handleSignal() {
	signal_chan := make(chan os.Signal, 1)
	signal.Notify(signal_chan, syscall.SIGINT, syscall.SIGTERM)
	<-signal_chan
	log.Printf("Exit")
	os.Exit(0)
}

func main() {
	go handleSignal()
	flag.Parse()
	full_addr := fmt.Sprintf("%s:%d", *addr, *port)
	conn, err := grpc.Dial(full_addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewClipClient(conn)
	clipClient, err := NewClipClient(&c)
	if err != nil {
		log.Fatalf("failed to create clip client: %v", err)
	}
	// go clipClient.KeepAlive()
	go clipClient.WatchClipboardSend()
	go clipClient.ConnectRecvUpdate()
	log.Printf("Client %s is ready", *hostname)
	select {}
}
