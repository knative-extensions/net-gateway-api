module knative.dev/net-ingressv2

go 1.15

require (
	github.com/google/go-cmp v0.5.6
	github.com/gorilla/websocket v1.4.2
	google.golang.org/grpc v1.38.0
	k8s.io/api v0.21.0
	k8s.io/apimachinery v0.21.0
	k8s.io/client-go v0.21.0
	k8s.io/utils v0.0.0-20210305010621-2afb4311ab10
	knative.dev/hack v0.0.0-20210614141220-66ab1a098940
	knative.dev/networking v0.0.0-20210615114921-e291c8011a20
	knative.dev/pkg v0.0.0-20210616195222-841aa7369ca1
	sigs.k8s.io/gateway-api v0.3.0
)
