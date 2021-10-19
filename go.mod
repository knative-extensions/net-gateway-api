module knative.dev/net-gateway-api

go 1.16

require (
	github.com/google/go-cmp v0.5.6
	github.com/gorilla/websocket v1.4.2
	google.golang.org/grpc v1.41.0
	k8s.io/api v0.21.4
	k8s.io/apimachinery v0.21.4
	k8s.io/client-go v0.21.4
	k8s.io/utils v0.0.0-20210305010621-2afb4311ab10
	knative.dev/hack v0.0.0-20211019034732-ced8ce706528
	knative.dev/networking v0.0.0-20211019132235-c8d647402afa
	knative.dev/pkg v0.0.0-20211019132235-ba2b2b1bf268
	sigs.k8s.io/gateway-api v0.3.0
)
