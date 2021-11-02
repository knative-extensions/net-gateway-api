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
	knative.dev/hack v0.0.0-20211101195839-11d193bf617b
	knative.dev/networking v0.0.0-20211101215640-8c71a2708e7d
	knative.dev/pkg v0.0.0-20211101212339-96c0204a70dc
	sigs.k8s.io/gateway-api v0.3.0
)
