package pubsub

import (
	"context"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsub/pstest"
	"github.com/maxim-nazarenko/fiskil-lms/internal/lms"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func StartServer(ctx context.Context, logger lms.Logger) *pstest.Server {
	// server part
	srv := pstest.NewServer()
	go func() {
		defer srv.Close()
		<-ctx.Done()
		logger.Info("stopping pubsub server")
	}()

	return srv
}

func NewClient(ctx context.Context, serverAddress, project string) (*pubsub.Client, error) {
	conn, err := grpc.Dial(serverAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client, err := pubsub.NewClient(ctx, project, option.WithGRPCConn(conn))
	if err != nil {
		return nil, err
	}
	return client, nil
}
