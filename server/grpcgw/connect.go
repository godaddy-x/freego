package grpcgw

import (
	"io"

	utils "github.com/godaddy-x/freego/core/str"
	pb "github.com/godaddy-x/freego/protocol/grpcgw"
	"google.golang.org/grpc/metadata"
)

func (g *GrpcGateway) SetOnStreamEnd(fn func(u int64, device string)) {
	g.onStreamEnd = fn
}

// PushToSubject sends c=300 with broadcast_key HMAC; canonical u is the target subject.
func (g *GrpcGateway) PushToSubject(u int64, device, route string, plain []byte) error {
	if u < 0 || device == "" || route == "" {
		return errStream("push target invalid")
	}
	n := pb.NewNonce()
	t := utils.UnixSecond()
	const p int64 = 0
	msg := "push"
	canonical := pb.CanonicalResponse(route, plain, n, t, p, u, pb.PushCode, msg)
	s := pb.HMACSHA256(utils.Str2Bytes(g.broadcastKey), canonical)
	return g.hub.send(u, device, &pb.GatewayResponse{
		R: route, D: plain, N: n, S: s, C: pb.PushCode, M: msg, T: t, P: p,
	})
}

func (s *gatewayService) Connect(stream pb.Gateway_ConnectServer) error {
	return s.gw.serveConnect(stream)
}

func (g *GrpcGateway) serveConnect(stream pb.Gateway_ConnectServer) error {
	ctx := stream.Context()
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return errStream("missing metadata")
	}
	dev := md.Get(pb.MetadataDeviceKey)
	if len(dev) == 0 || dev[0] == "" {
		return errStream("device metadata required")
	}
	device := dev[0]
	if len(device) > pb.MaxMetadataDeviceLen {
		return errStream("device metadata too long")
	}

	bind, err := stream.Recv()
	if err != nil {
		return err
	}
	if bind.R != pb.StreamBindRoute {
		return errStream("first frame must be StreamBind")
	}
	ack, err := g.handleCall(ctx, bind)
	if err != nil {
		return err
	}
	if ack == nil || ack.C != 200 {
		if ack != nil {
			if err := stream.Send(ack); err != nil {
				return err
			}
		}
		return nil
	}

	conn := g.hub.add(bind.U, device)
	defer func() {
		if g.onStreamEnd != nil {
			g.onStreamEnd(bind.U, device)
		}
		g.hub.remove(bind.U, device, conn)
	}()

	go g.streamWriter(stream, conn)
	if err := conn.send(ack); err != nil {
		return err
	}

	for {
		in, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		resp, err := g.handleCall(ctx, in)
		if err != nil {
			return err
		}
		if err := conn.send(resp); err != nil {
			return err
		}
	}
}

func (g *GrpcGateway) streamWriter(stream pb.Gateway_ConnectServer, conn *streamConn) {
	for msg := range conn.ch {
		if err := stream.Send(msg); err != nil {
			return
		}
	}
}

type errStream string

func (e errStream) Error() string { return string(e) }

const (
	errStreamReplaced errStream = "stream replaced"
	errStreamFull     errStream = "stream send queue full"
)
