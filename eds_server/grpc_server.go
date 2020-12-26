package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"sync"
	"sync/atomic"
	"time"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"

	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"

	serverv3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"

	"golang.org/x/net/http2"
)

var (
	debug bool

	port        uint
	httpport    string
	gatewayPort uint

	mode string

	version int32

	cache    cachev3.SnapshotCache
	strSlice = []string{}
	//strSlice = []string{"127.0.0.1:8081"}
)

const (
	Eds         = "eds"
	clusterName = "myservice"
)

func init() {
	flag.BoolVar(&debug, "debug", true, "Use debug logging")
	flag.UintVar(&port, "port", 8080, "Management server port")
	flag.StringVar(&httpport, "httpport", ":5000", "HTTP EDS Registration Port")
	flag.StringVar(&mode, "eds", Eds, "Management server type (eds only now)")
}

func (cb *Callbacks) Report() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	log.WithFields(log.Fields{"fetches": cb.Fetches, "requests": cb.Requests}).Info("cb.Report()  callbacks")
}
func (cb *Callbacks) OnStreamOpen(_ context.Context, id int64, typ string) error {
	log.Infof("OnStreamOpen %d open for %s", id, typ)
	return nil
}
func (cb *Callbacks) OnStreamClosed(id int64) {
	log.Infof("OnStreamClosed %d closed", id)
}
func (cb *Callbacks) OnStreamRequest(id int64, r *discoverygrpc.DiscoveryRequest) error {
	log.Infof("OnStreamRequest %v", r.TypeUrl)

	// each envoy asks for a specific resource type.  In this case its the clusterloadassignement
	// for cluster "myservice"
	log.Infof("OnStreamRequest ResourceNames %v", r.ResourceNames)
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.Requests++
	if cb.Signal != nil {
		close(cb.Signal)
		cb.Signal = nil
	}
	return nil
}
func (cb *Callbacks) OnStreamResponse(int64, *discoverygrpc.DiscoveryRequest, *discoverygrpc.DiscoveryResponse) {
	log.Infof("OnStreamResponse...")
	cb.Report()
}
func (cb *Callbacks) OnFetchRequest(ctx context.Context, req *discoverygrpc.DiscoveryRequest) error {
	log.Infof("OnFetchRequest...")
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.Fetches++
	if cb.Signal != nil {
		close(cb.Signal)
		cb.Signal = nil
	}
	return nil
}
func (cb *Callbacks) OnFetchResponse(*discoverygrpc.DiscoveryRequest, *discoverygrpc.DiscoveryResponse) {
	log.Infof("OnFetchResponse...")
}

type Callbacks struct {
	Signal   chan struct{}
	Debug    bool
	Fetches  int
	Requests int
	mu       sync.Mutex
}

const grpcMaxConcurrentStreams = 1000000

// RunManagementServer starts an xDS server at the given port.
func RunManagementServer(ctx context.Context, server serverv3.Server, port uint) {
	var grpcOptions []grpc.ServerOption
	grpcOptions = append(grpcOptions, grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams))
	grpcServer := grpc.NewServer(grpcOptions...)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.WithError(err).Fatal("failed to listen")
	}

	// register services
	endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, server)

	log.WithFields(log.Fields{"port": port}).Info("management server listening")
	go func() {
		if err = grpcServer.Serve(lis); err != nil {
			log.Error(err)
		}
	}()
	<-ctx.Done()

	grpcServer.GracefulStop()
}

func registerhendpoint(w http.ResponseWriter, r *http.Request) {
	keys, ok := r.URL.Query()["endpoint"]

	if !ok || len(keys[0]) < 1 {
		log.Println("Url Param 'key' is missing")
		return
	}

	key := keys[0]

	for _, b := range strSlice {
		if b == key {
			fmt.Fprint(w, fmt.Sprintf("%s already registered", key))
			return
		}
	}

	strSlice = append(strSlice, key)
	fmt.Fprint(w, fmt.Sprintf("%s ok", key))
}

func deregisterhendpoint(w http.ResponseWriter, r *http.Request) {
	keys, ok := r.URL.Query()["endpoint"]

	if !ok || len(keys[0]) < 1 {
		log.Println("Url Param 'key' is missing")
		return
	}

	key := keys[0]

	for i, v := range strSlice {
		if v == key {
			strSlice = append(strSlice[:i], strSlice[i+1:]...)
			break
		}
	}

	fmt.Fprint(w, fmt.Sprintf("%s ok", key))
}

func main() {
	flag.Parse()

	log.SetLevel(log.DebugLevel)

	ctx := context.Background()

	http.HandleFunc("/edsservice/register", registerhendpoint)
	http.HandleFunc("/edsservice/deregister", deregisterhendpoint)

	hsrv := &http.Server{
		Addr: httpport,
	}
	http2.ConfigureServer(hsrv, &http2.Server{})
	go hsrv.ListenAndServe()

	log.Printf("Starting control plane")

	signal := make(chan struct{})
	cb := &Callbacks{
		Signal:   signal,
		Fetches:  0,
		Requests: 0,
	}
	cache = cachev3.NewSnapshotCache(true, cachev3.IDHash{}, nil)

	srv := serverv3.NewServer(ctx, cache, cb)

	// start the xDS server
	go RunManagementServer(ctx, srv, port)

	<-signal

	for {
		nodeId := cache.GetStatusKeys()[0]

		var locEndpoints []*endpoint.LocalityLbEndpoints
		for _, v := range strSlice {

			h, p, err := net.SplitHostPort(v)
			if err != nil {
				log.Fatalf("Cloud not split host/port %v", err)
			}
			uport, err := strconv.ParseUint(p, 0, 64)

			if err != nil {
				log.Fatalf("Cloud not convert port %v", err)
			}

			log.Infof(">>>>>>>>>>>>>>>>>>> creating cluster, remoteHost, nodeID %s,  %s, %s", clusterName, v, nodeId)

			ep := []*endpoint.LbEndpoint{
				{
					HostIdentifier: &endpoint.LbEndpoint_Endpoint{
						Endpoint: &endpoint.Endpoint{
							Address: &core.Address{
								Address: &core.Address_SocketAddress{
									SocketAddress: &core.SocketAddress{
										Protocol: core.SocketAddress_TCP,
										Address:  h,
										PortSpecifier: &core.SocketAddress_PortValue{
											PortValue: uint32(uport),
										},
									},
								},
							},
						},
					},
				},
			}

			locEndpoints = append(locEndpoints, &endpoint.LocalityLbEndpoints{
				LbEndpoints: ep,
			})
		}

		log.Infof("%v", locEndpoints)

		c := []types.Resource{&endpoint.ClusterLoadAssignment{
			ClusterName: clusterName,
			Endpoints:   locEndpoints,
		}}
		// =================================================================================
		atomic.AddInt32(&version, 1)
		log.Infof(">>>>>>>>>>>>>>>>>>> creating snapshot Version " + fmt.Sprint(version))

		snap := cachev3.NewSnapshot(fmt.Sprint(version), c, nil, nil, nil, nil, nil)
		err := cache.SetSnapshot(nodeId, snap)
		if err != nil {
			log.Fatalf("Could not set snapshot %v", err)
		}

		//reader := bufio.NewReader(os.Stdin)
		//_, _ = reader.ReadString('\n')

		time.Sleep(60 * time.Second)

	}

}
