package controller

import (
	"context"

	"github.com/acorn-io/acorn-linkerd-plugin/pkg/scheme"
	"github.com/acorn-io/baaah"
	"k8s.io/client-go/kubernetes"
)

type Options struct {
	Clientset *kubernetes.Clientset

	DebugImage string
}

func Start(ctx context.Context, opt Options) error {
	router, err := baaah.DefaultRouter("linkerd-controller", scheme.Scheme)
	if err != nil {
		return err
	}

	RegisterRoutes(router, opt.Clientset, opt.DebugImage)

	return router.Start(ctx)
}
