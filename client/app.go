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
	"math"
	"os"
	"strings"
	"sync"
	"time"
)

const DefaultFramesPerSecond = 60

type App struct {
	Instance         string
	ReapiHost        string
	LastReapiLatency time.Duration
	CA               string
	Done             bool
	Conn             *grpc.ClientConn
	workerConns      map[string]*grpc.ClientConn
	Ops              map[string]*longrunning.Operation
	Actions          map[string]*reapi.Action
	Metadatas        map[string]*reapi.RequestMetadata
	Invocations      map[string][]string
	Fetches          uint
	Mutex            *sync.Mutex

	FramesPerSecond  int
	UpdatesPerSecond float64
	UpdateCountdown  int
}

func NewApp(reapiHost string, ca string) *App {
	return &App{
		Instance:         "shard",
		ReapiHost:        reapiHost,
		CA:               ca,
		Ops:              make(map[string]*longrunning.Operation),
		Actions:          make(map[string]*reapi.Action),
		Metadatas:        make(map[string]*reapi.RequestMetadata),
		Invocations:      make(map[string][]string),
		workerConns:      make(map[string]*grpc.ClientConn),
		Mutex:            &sync.Mutex{},
		FramesPerSecond:  DefaultFramesPerSecond,
		UpdatesPerSecond: DefaultFramesPerSecond,
	}
}

func ValidUpdatesPerSecond(ups float64) bool {
	return ups > 0 && !math.IsInf(ups, 1)
}

func (a *App) SetRates(fps int, ups float64) {
	ups = min(float64(fps), ups)
	if fps == a.FramesPerSecond && ups == a.UpdatesPerSecond {
		return
	}
	a.FramesPerSecond = fps
	a.UpdatesPerSecond = ups
	a.UpdateCountdown = 0
}

func (a *App) FramesToSkip() int {
	// Round the frames between updates up so the update rate stays within the limit.
	frames := math.Ceil(float64(a.FramesPerSecond) / a.UpdatesPerSecond)
	// A very small update rate can exceed the countdown's integer range.
	if frames >= float64(math.MaxInt) {
		return math.MaxInt
	}
	return int(frames) - 1
}

func (a *App) GetWorkerConn(worker string, ca string) *grpc.ClientConn {
	if a.workerConns[worker] == nil {
		a.workerConns[worker] = connect(worker, ca)
	}
	return a.workerConns[worker]
}

func (a *App) Connect() {
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
