module knative.dev/net-gateway-api

go 1.16

require (
	github.com/google/go-cmp v0.5.6
	github.com/gorilla/websocket v1.4.2
	go.uber.org/zap v1.19.1
	google.golang.org/grpc v1.42.0
	k8s.io/api v0.23.5
	k8s.io/apimachinery v0.23.5
	k8s.io/client-go v0.23.5
	k8s.io/code-generator v0.23.5
	k8s.io/utils v0.0.0-20220210201930-3a6ce19ff2f9
	knative.dev/hack v0.0.0-20220401031746-a75ca495e7f4
	knative.dev/networking v0.0.0-20220401171946-0aef61efb50c
	knative.dev/pkg v0.0.0-20220401214546-c2f1f3ec291f
	sigs.k8s.io/gateway-api v0.4.0
	sigs.k8s.io/yaml v1.3.0
)
