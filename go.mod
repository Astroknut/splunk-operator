module github.com/splunk/splunk-operator

go 1.16

require (
	github.com/aws/aws-sdk-go v1.42.16
	github.com/go-logr/logr v0.4.0
	github.com/google/go-cmp v0.5.6
	github.com/minio/minio-go/v7 v7.0.16
	github.com/nbutton23/zxcvbn-go v0.0.0-20210217022336-fa2cb2858354 // indirect
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.17.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.0
	github.com/securego/gosec v0.0.0-20200401082031-e946c8c39989 // indirect
	go.uber.org/zap v1.19.0
	golang.org/x/crypto v0.0.0-20210322153248-0c34fe9e7dc2 // indirect
	golang.org/x/sys v0.0.0-20220114195835-da31bd327af9 // indirect
	golang.org/x/tools v0.1.8 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	k8s.io/api v0.22.4
	k8s.io/apiextensions-apiserver v0.22.1
	k8s.io/apimachinery v0.22.4
	k8s.io/client-go v0.22.4
	k8s.io/kubectl v0.22.4
	sigs.k8s.io/controller-runtime v0.10.0
)
