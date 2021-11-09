module knative.dev/net-gateway-api

go 1.16

require (
	github.com/google/go-cmp v0.5.6
	github.com/gorilla/websocket v1.4.2
	go.uber.org/zap v1.19.1
	google.golang.org/grpc v1.42.0
	k8s.io/api v0.21.4
	k8s.io/apimachinery v0.21.4
	k8s.io/client-go v0.21.4
	k8s.io/code-generator v0.21.4
	k8s.io/utils v0.0.0-20210305010621-2afb4311ab10
	knative.dev/hack v0.0.0-20211108170701-96aac1c30be3
	knative.dev/networking v0.0.0-20211109014842-5d4dc7ffa17b
	knative.dev/pkg v0.0.0-20211108064904-3cc697a3cb09
	sigs.k8s.io/gateway-api v0.3.0
	sigs.k8s.io/yaml v1.3.0
)
