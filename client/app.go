package client

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	reapi "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"google.golang.org/genproto/googleapis/longrunning"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"os"
	"strings"
	"sync"
	"time"
)

type App struct {
	Instance         string
	RedisHost        string
	ReapiHost        string
	LastRedisLatency time.Duration
	LastReapiLatency time.Duration
	CA               string
	Done             bool
	Client           *UnifiedRedis
	Conn             *grpc.ClientConn
	workerConns      map[string]*grpc.ClientConn
	Ops              map[string]*longrunning.Operation
	Metadatas        map[string]*reapi.RequestMetadata
	Invocations      map[string][]string
	Fetches          uint
	Mutex            *sync.Mutex

	FrameLimit      int
	SkipFrames      int
	UpdateCountdown int
}

func NewApp(redisHost string, reapiHost string, ca string) *App {
	return &App{
		Instance:    "shard",
		RedisHost:   redisHost,
		ReapiHost:   reapiHost,
		CA:          ca,
		Done:        false,
		Ops:         make(map[string]*longrunning.Operation),
		Metadatas:   make(map[string]*reapi.RequestMetadata),
		Invocations: make(map[string][]string),
		workerConns: make(map[string]*grpc.ClientConn),
		Client:      &UnifiedRedis{},
		Mutex:       &sync.Mutex{},
		FrameLimit:  60,
	}
}

func (a *App) GetWorkerConn(worker string, ca string) *grpc.ClientConn {
	if a.workerConns[worker] == nil {
		a.workerConns[worker] = connect(worker, ca)
	}
	return a.workerConns[worker]
}

func (a *App) Connect() {
	a.Client.connect(a.RedisHost)
	a.Conn = connect(a.ReapiHost, a.CA)
}

func connect(host string, ca string) *grpc.ClientConn {
	var opts []grpc.DialOption
	if strings.HasPrefix(host, "grpcs://") {
		host = host[8:]
		if !strings.Contains(host, ":") {
			host = host + ":443"
		}
		creds, err := loadTLSCredentials(ca)
		if err != nil {
			panic(err)
		}
		opts = []grpc.DialOption{grpc.WithTransportCredentials(creds)}
	} else {
		opts = []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	}
	conn, err := grpc.Dial(host, opts...)
	if err != nil {
		panic(err)
	}
	return conn
}

func loadTLSCredentials(ca string) (credentials.TransportCredentials, error) {
	if ca == "" {
		return credentials.NewTLS(&tls.Config{}), nil
	}
	// Load certificate of the CA who signed server's certificate
	pemServerCA, err := os.ReadFile(ca)
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(pemServerCA) {
		return nil, fmt.Errorf("failed to add server CA's certificate")
	}

	// Create the credentials and return it
	config := &tls.Config{
		RootCAs: certPool,
	}

	return credentials.NewTLS(config), nil
}
