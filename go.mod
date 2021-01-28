module knative.dev/net-ingressv2

go 1.14

require (
	github.com/google/licenseclassifier v0.0.0-20200708223521-3d09a0ea2f39
	github.com/sergi/go-diff v1.1.0 // indirect
	k8s.io/apimachinery v0.19.7
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	k8s.io/code-generator v0.19.7
	k8s.io/kube-openapi v0.0.0-20200831175022-64514a1d5d59
	knative.dev/hack v0.0.0-20210120165453-8d623a0af457
	knative.dev/networking v0.0.0-20210125050654-94433ab7f620
	knative.dev/pkg v0.0.0-20210124203454-7101e9d4f6c6
	sigs.k8s.io/service-apis v0.1.0
)

replace (
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.9.2

	k8s.io/api => k8s.io/api v0.19.2
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.19.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.19.2
	k8s.io/apiserver => k8s.io/apiserver v0.19.2
	k8s.io/client-go => k8s.io/client-go v0.19.2
	k8s.io/code-generator => k8s.io/code-generator v0.19.2
)
