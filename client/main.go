package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/spiffe/go-spiffe/v2/proto/spiffe/workload"
	"google.golang.org/grpc"
)

const (
	pipeName = `\\.\pipe\wservice`
)

func main() {
	conn, err := grpc.Dial(pipeName, grpc.WithInsecure(), grpc.WithContextDialer(
		winio.DialPipeContext,
	))
	if err != nil {
		log.Fatalf("failed to dial: %+v", err)
	}
	defer conn.Close()

	log.Println("PID:", os.Getpid())

	client := workload.NewSpiffeWorkloadAPIClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := client.FetchX509SVID(ctx, &workload.X509SVIDRequest{})
	if err != nil {
		log.Fatalf("failed to fetch SVID: %v", err)
	}

	resp, err := stream.Recv()
	if err != nil {
		log.Fatalf("failed to call recv: %v", err)
	}
	log.Printf("SPIFFEID: %q", resp.Svids[0].SpiffeId)
}
