package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"sync"

	log "github.com/sirupsen/logrus"

	pb "github.com/Roarcannotprogramming/clipipe/proto"

	"google.golang.org/grpc"
)

type HostClip struct {
	Host string
	Clip []byte
}

type Clients struct {
	Named_chans map[string]chan HostClip
	mu          sync.Mutex
}

var (
	port      = flag.Int("port", 8799, "The server port")
	max_saved = flag.Int("max_saved", 10, "The max number of saved clipboard")
	loglevel  = flag.String("loglevel", "info", "The log level")
	clients   = Clients{Named_chans: make(map[string]chan HostClip)}
)

type server struct {
	pb.UnimplementedClipServer
}

func (s *server) Ping(ctx context.Context, in *pb.PingRequest) (*pb.PingResponse, error) {
	if in.Status.Code == pb.Code_OK && in.Status.Message == "ping" {
		out := &pb.PingResponse{
			Status: &pb.Status{
				Code:    pb.Code_OK,
				Message: "pong",
			},
			Id: in.Id,
		}
		return out, nil
	}
	return nil, fmt.Errorf("invalid ping request")
}

func (s *server) Push(ctx context.Context, in *pb.PushRequest) (*pb.PushResponse, error) {
	if in.Status.Code == pb.Code_OK && in.Status.Message == "push" {
		log.Debugf("Received from %s: %s", in.Id, in.Msg)
		clients.mu.Lock()
		for id, hc := range clients.Named_chans {
			if id != in.Id {
				hc <- HostClip{Host: in.Id, Clip: in.Msg}
			}
		}
		clients.mu.Unlock()
		out := &pb.PushResponse{
			Status: &pb.Status{
				Code:    pb.Code_OK,
				Message: "done",
			},
			Id: in.Id,
		}
		return out, nil
	}
	return nil, fmt.Errorf("invalid push request")
}

func (s *server) GetStream(in *pb.ConnRequest, stream pb.Clip_GetStreamServer) error {
	if in.Status.Code == pb.Code_OK && in.Status.Message == "connect" {
		clients.mu.Lock()
		if _, exists := clients.Named_chans[in.Id]; !exists {
			clients.Named_chans[in.Id] = make(chan HostClip, *max_saved)
		} else {
			log.Fatalf("Client %s already exists", in.Id)
		}
		clients.mu.Unlock()
		log.Infof("Client %s connected", in.Id)
		for {
			select {
			case tmp := <-clients.Named_chans[in.Id]:
				log.Debugf("Sending to %s: %s", in.Id, tmp.Clip)
				err := stream.Send(&pb.MsgResponse{
					Id: tmp.Host,
					Status: &pb.Status{
						Code:    pb.Code_OK,
						Message: "done",
					},
					Msg: tmp.Clip,
				})
				if err != nil {
					log.Errorf("Error sending to %s: %v", in.Id, err)
					clients.mu.Lock()
					delete(clients.Named_chans, in.Id)
					clients.mu.Unlock()
					return err
				}
			case <-stream.Context().Done():
				log.Infof("Client %s disconnected", in.Id)
				clients.mu.Lock()
				delete(clients.Named_chans, in.Id)
				clients.mu.Unlock()
				return nil
			}
		}
	}
	return fmt.Errorf("invalid get request")
}

func main() {
	flag.Parse()
	switch *loglevel {
	case "trace":
		log.SetLevel(log.TraceLevel)
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	case "fatal":
		log.SetLevel(log.FatalLevel)
	case "panic":
		log.SetLevel(log.PanicLevel)
	default:
		log.Fatalf("Invalid log level: %s", *loglevel)
	}
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterClipServer(s, &server{})
	log.Infof("Server listening on port %d", *port)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
