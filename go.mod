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
	k8s.io/utils v0.0.0-20210820185131-d34e5cb4466e
	knative.dev/hack v0.0.0-20220224013837-e1785985d364
	knative.dev/networking v0.0.0-20220302014942-9ba6cf46d6dc
	knative.dev/pkg v0.0.0-20220301181942-2fdd5f232e77
	sigs.k8s.io/gateway-api v0.4.0
	sigs.k8s.io/yaml v1.3.0
)
