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
	knative.dev/hack v0.0.0-20211105231158-29f86c2653b5
	knative.dev/networking v0.0.0-20211104064801-6871f98f7b4d
	knative.dev/pkg v0.0.0-20211104101302-51b9e7f161b4
	sigs.k8s.io/gateway-api v0.3.0
	sigs.k8s.io/yaml v1.3.0
)
