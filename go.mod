module knative.dev/net-gateway-api

go 1.16

require (
	github.com/google/go-cmp v0.5.6
	github.com/gorilla/websocket v1.4.2
	go.uber.org/zap v1.19.1
	google.golang.org/grpc v1.42.0
	k8s.io/api v0.22.5
	k8s.io/apimachinery v0.22.5
	k8s.io/client-go v0.22.5
	k8s.io/code-generator v0.22.5
	k8s.io/utils v0.0.0-20210819203725-bdf08cb9a70a
	knative.dev/hack v0.0.0-20220118141833-9b2ed8471e30
	knative.dev/networking v0.0.0-20220117015928-52fb6ee37bf9
	knative.dev/pkg v0.0.0-20220118160532-77555ea48cd4
	sigs.k8s.io/gateway-api v0.3.0
	sigs.k8s.io/yaml v1.3.0
)
