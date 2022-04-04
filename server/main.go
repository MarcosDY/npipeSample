package main

import (
	"context"
	"errors"
	"log"
	"net"
	"syscall"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"github.com/spiffe/go-spiffe/v2/proto/spiffe/workload"
	"github.com/zeebo/errs"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

const (
	pipeName = `\\.\pipe\wservice`
)

var (
	kernel32                        = syscall.NewLazyDLL("kernel32.dll")
	getNamedPipeClientProcessIdFunc = kernel32.NewProc("GetNamedPipeClientProcessId")
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatalf("server finished: %v\n", err)
	}
}

func run(ctx context.Context) (err error) {
	listener, err := winio.ListenPipe(pipeName, nil)
	if err != nil {
		return errs.Wrap(err)
	}
	defer listener.Close()

	server := grpc.NewServer(grpc.Creds(new(TransportCredentials)))
	workload.RegisterSpiffeWorkloadAPIServer(server, &Server{})
	log.Printf("Listening on %s", pipeName)
	return server.Serve(listener)
}

type Server struct {
	workload.SpiffeWorkloadAPIServer
}

func (s *Server) FetchX509SVID(req *workload.X509SVIDRequest, stream workload.SpiffeWorkloadAPI_FetchX509SVIDServer) error {
	ctx := stream.Context()

	p, ok := peer.FromContext(ctx)
	if !ok {
		return status.Error(codes.Internal, "no peer on context")
	}
	authInfo, ok := p.AuthInfo.(*PipeAuthInfo)
	if !ok {
		return status.Errorf(codes.Internal, "unexpected pipe info: %T", p.AuthInfo)
	}

	pID, err := getProcessId(authInfo.PHandle())
	if err != nil {
		return status.Errorf(codes.Internal, "failed to get PID: %v", err)
	}

	log.Printf("ProcessID: %d\n", pID)

	return stream.Send(&workload.X509SVIDResponse{
		Svids: []*workload.X509SVID{
			{
				SpiffeId: "someID",
			},
		},
	})
}

type TransportCredentials struct {
}

func (c *TransportCredentials) ClientHandshake(context.Context, string, net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return nil, nil, errors.New("invalid connection")
}

func (c *TransportCredentials) ServerHandshake(conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	type Fder interface {
		Fd() uintptr
	}
	fder, ok := conn.(Fder)
	if !ok {
		conn.Close()
		return nil, nil, errors.New("invalid conenction")
	}

	return conn, newPipeAuthInfo(fder.Fd()), nil
}

func (c *TransportCredentials) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{
		SecurityProtocol: "spire-attestation",
		SecurityVersion:  "0.2",
		ServerName:       "spire-agent",
	}
}

func (c *TransportCredentials) Clone() credentials.TransportCredentials {
	clone := *c
	return &clone
}

func (c *TransportCredentials) OverrideServerName(string) error {
	return nil
}

// TODO: it must be implemented from peertracker
type PipeAuthInfo struct {
	pHandle uintptr
}

func newPipeAuthInfo(pHandle uintptr) *PipeAuthInfo {
	return &PipeAuthInfo{
		pHandle: pHandle,
	}
}

func (p *PipeAuthInfo) AuthType() string {
	return "pipe"
}

func (p *PipeAuthInfo) PHandle() uintptr {
	return p.pHandle
}

func getProcessId(pHandle uintptr) (uint32, error) {
	var pid uint32
	r1, _, err := getNamedPipeClientProcessIdFunc.Call(pHandle, uintptr(unsafe.Pointer(&pid)))
	if r1 == 0 {
		return 0, errs.New("GetNamedPipeClientProcessId: %v", err)
	}
	return pid, nil
}
