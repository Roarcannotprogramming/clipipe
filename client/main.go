package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"golang.design/x/clipboard"

	pb "github.com/Roarcannotprogramming/clipipe/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	addr = flag.String("addr", "127.0.0.1", "The server address")
	port = flag.Int("port", 8799, "The server port")
)

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
	for {
		new_data := clipboard.Watch(cc.ctx, clipboard.FmtText)
		d := <-new_data
		cc.mu.Lock()
		cc.Content = d
		cc.mu.Unlock()
		data <- d
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
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}
	return hostname, nil
}

func (cclient *ClipClient) KeepAlive() {
	for {
		hostname, err := GetHostname()
		if err != nil {
			log.Fatalf("failed to get hostname: %v", err)
		}
		ping_ctx, cancel := context.WithTimeout(cclient.pb_ctx, time.Second)
		r, ping_err := (*cclient.pb).Ping(ping_ctx, &pb.PingRequest{Id: hostname})
		if ping_err != nil {
			log.Fatalf("failed to ping: %v", ping_err)
		}
		cancel()
		log.Printf("Ping response: %s", r.Id)
		time.Sleep(5 * time.Second)
	}
}

func main() {
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
	go clipClient.KeepAlive()
	// Contact the server and print out its response.
	time.Sleep(100 * time.Second)
}
