package controller

import (
	"context"

	"github.com/acorn-io/acorn-linkerd-plugin/pkg/scheme"
	"github.com/acorn-io/baaah"
	"k8s.io/client-go/kubernetes"
)

type Options struct {
	K8s kubernetes.Interface

	DebugImage    string
	ClusterDomain string

	IngressEndpointName      string
	IngressEndpointNamespace string
}

func Start(ctx context.Context, opt Options) error {
	router, err := baaah.DefaultRouter("linkerd-controller", scheme.Scheme)
	if err != nil {
		return err
	}

	if err := RegisterRoutes(router, opt.K8s, opt.DebugImage, opt.ClusterDomain, opt.IngressEndpointName, opt.IngressEndpointNamespace); err != nil {
		return err
	}

	return router.Start(ctx)
}
