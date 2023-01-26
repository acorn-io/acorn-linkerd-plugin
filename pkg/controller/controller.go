package controller

import (
	"context"

	"github.com/acorn-io/acorn-linkerd-plugin/pkg/scheme"
	"github.com/acorn-io/baaah"
	"github.com/acorn-io/baaah/pkg/restconfig"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

func Start(ctx context.Context) error {
	config, err := restconfig.Default()
	if err != nil {
		return err
	}
	config.APIPath = "api"
	config.GroupVersion = &corev1.SchemeGroupVersion
	config.NegotiatedSerializer = scheme.Codecs

	clientset := kubernetes.NewForConfigOrDie(config)

	router, err := baaah.DefaultRouter("linkerd-controller", scheme.Scheme)
	if err != nil {
		return err
	}

	RegisterRoutes(router, clientset)

	return router.Start(ctx)
}
