# Envoy EDS "hello world"

A simple app demonstrating a small part of [Envoy's Endpoint Discovery Service](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/eds.proto#envoy-api-file-envoy-api-v2-eds-proto). 

Some of the configurations are hardcoded in the `envoy_config.yaml` file just as a demonstration.  Specifically, the service, cluster and bootstrap endpoint
to get discovery information.


## References
 - [Endpoint Discovery Service](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/endpoint/endpoint)
 - [Endpoing Overview](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/service_discovery#arch-overview-service-discovery-types-sds)
 - [SDS at Lyft](https://github.com/lyft/discovery)
 - [Envoy Dynamic Configuration](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/dynamic_configuration.html)
 - [Envoy API for developers](https://github.com/envoyproxy/data-plane-api/blob/master/API_OVERVIEW.md)

### Prerequsites

- [envoy binary ](https://envoyproxy.io)
- python (and virtualenv)
- golang

Overall there are three components:

1. Envoy server (listen  port `:10000`)
   This is the core proxy that will startup without any upstream endpoints.  It will ask the EDS server for valid endpoints for a given cluster
2. EDS gRPC server (listen port: `:8080`).  This gRPC server returns a list of endpoints that it was told about back to envoy
3. Upstream Endpoints (listen ports `:8081`, `:8082`).  THese are the actual upstream webservers that envoy will ultimately connect to.  On startup, these python webservers will 'tell' the GRPC server of their existence and later on the grpc server will inform envoy.

![arch.png](images/arch.png)

---

### Start EDS Server

```bash
cd eds_server
go run grpc_server.go
```

## Start Envoy with SDS


Get Envoy binary

```bash
docker cp `docker create envoyproxy/envoy-dev:latest`:/usr/local/bin/envoy .
```

So start envoy with debug enabled:

```bash
./envoy -c envoy_config.yaml -l debug
```

At this point, envoy attempts to connect to the upstream EDS gRPC cluster at `127.0.0.1:8080` but since your EDS isn't running yet, nothing additional config takes place.


```bash
 curl -v  http://localhost:10000/

> GET / HTTP/1.1
> Host: localhost:10000
> User-Agent: curl/7.72.0
> Accept: */*
> 

< HTTP/1.1 503 Service Unavailable
< content-length: 19
< content-type: text/plain
< date: Fri, 25 Dec 2020 13:26:05 GMT
< server: envoy
< 

no healthy upstream
```


As mentioned, this is the gRPC server that envoy will connect to.  Since the EDS server doesn't 'know' about any other webservers, its list of endpoints is blank

When envoy contacts the EDS server, it will return an empty list

The following shows the EDS Server returning a cache snapshot back to envoy

```bash
$ go run grpc_server.go 
INFO[0000] Starting control plane                       
INFO[0000] management server listening                   port=8080
INFO[0022] OnStreamOpen 1 open for type.googleapis.com/envoy.config.endpoint.v3.ClusterLoadAssignment 
INFO[0022] OnStreamRequest type.googleapis.com/envoy.config.endpoint.v3.ClusterLoadAssignment 
INFO[0022] OnStreamRequest ResourceNames [myservice]    
INFO[0022] []                                           
INFO[0022] >>>>>>>>>>>>>>>>>>> creating snapshot Version 1 
INFO[0022] OnStreamResponse...                          
INFO[0022] cb.Report()  callbacks                        fetches=0 requests=1
INFO[0022] OnStreamRequest type.googleapis.com/envoy.config.endpoint.v3.ClusterLoadAssignment 
INFO[0022] OnStreamRequest ResourceNames [myservice]    

```

however, the cache doesn't contain any endpoints so envoy can't proxy to any webserver (clearly since no upstream server is even running!)


## Start Upstream services

Now in a new window, start the upstream service on a given the default port for the script (```:8081```)

```bash
cd upstream/

virtualenv env --python=/usr/bin/python3.7
source env/bin/activate
pip install -r requirements.txt

$ python3 server.py -p 8081
```

On startup, the webserver will make a rest API call back to the EDS server over HTTP just to let it know its alive and the host/port it listens on.

>> NOTE:

This is important bit:  I just happened to use the endpoint webservers startup stage to let EDS know about its existence...you can use **ANY** technique..that bit is not specified..

eg, the EDS Server runs both grpc on port `:8080` AND and HTTP server on port `:5000`.  The HTTP server is simply a way for upstream servers to 'register' itself...

```python
def main(argv):
   port = 18080
   print("Registering endpoint: 127.0.0.1:", port)
   url = 'http://localhost:5000/edsservice/register?endpoint=127.0.0.1:' + port
   f = urllib.request.urlopen(url)
   print(f.read().decode('utf-8'))
```

The REST endpoint on the EDS server `/edsservice/register?endpoint=` is something i just made up


You'll see that the EDS server's next snapshot contains the  host/port back to envoy:

```log
INFO[0382] OnStreamRequest type.googleapis.com/envoy.config.endpoint.v3.ClusterLoadAssignment 
INFO[0382] OnStreamRequest ResourceNames [myservice]    
INFO[0442] >>>>>>>>>>>>>>>>>>> creating cluster, remoteHost, nodeID myservice,  127.0.0.1:8081, test-id 
INFO[0442] [lb_endpoints:{endpoint:{address:{socket_address:{address:"127.0.0.1" port_value:8081}}}}] 
INFO[0442] >>>>>>>>>>>>>>>>>>> creating snapshot Version 8 
INFO[0442] OnStreamResponse...                          
INFO[0442] cb.Report()  callbacks 
```

### Check client connectivity via envoy

Since we already started the upstream service above, you can connect to it via envoy:

```bash
$ curl -v  http://localhost:10000/
 
< HTTP/1.1 200 OK
< content-type: text/html; charset=utf-8
< content-length: 36
< server: envoy
< date: Mon, 30 Apr 2018 06:21:43 GMT
< x-envoy-upstream-service-time: 3
< 
* Connection #0 to host localhost left intact
40b9bc6f-77b8-49b7-b939-1871507b0fcc
```

(note the `server: envoy` part in the header)

Note, if you can also directly remove an upstream host:port from EDS by invoking the deregister :

 `curl http://localhost:5000/edsservice/deregister?endpoint=127.0.0.1:8081`


## Rinse and repeat

Ok, you can continue to play with the endpoints by adding and adding or stopping new upstream services on different ports:

eg:

```bash
$ python server.py -p 8082
$ python server.py -p 8083
```

each successive calls to envoy should show the various endpoints
## Conclusion

I wrote this up just in an effort to play around with `envoy` i'm pretty much new to this so i likely have numerous 
misunderstanding on what i just did here...if you see something amiss, please do let me know.

