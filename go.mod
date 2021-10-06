module knative.dev/net-ingressv2

go 1.16

require (
	github.com/google/go-cmp v0.5.6
	github.com/gorilla/websocket v1.4.2
	google.golang.org/grpc v1.41.0
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	k8s.io/utils v0.0.0-20210820185131-d34e5cb4466e
	knative.dev/hack v0.0.0-20210806075220-815cd312d65c
	knative.dev/networking v0.0.0-20211001132006-8da57ce00036
	knative.dev/pkg v0.0.0-20211005203644-a00ba487121e
	sigs.k8s.io/gateway-api v0.4.0-rc1
)
