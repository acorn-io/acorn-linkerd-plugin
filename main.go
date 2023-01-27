package main

import (
	"flag"
	"fmt"

	"github.com/acorn-io/acorn-linkerd-plugin/pkg/controller"
	"github.com/acorn-io/acorn-linkerd-plugin/pkg/scheme"
	"github.com/acorn-io/acorn-linkerd-plugin/pkg/version"
	"github.com/acorn-io/baaah/pkg/restconfig"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

var (
	versionFlag = flag.Bool("version", false, "print version")

	debugImageFlag = flag.String("debug-image", "ghcr.io/acorn/acorn-linkerd-plugin:main", "the image to use for killing linkerd sidecar")

	clusterDomain = flag.String("cluster-domain", "cluster.local", "The cluster domain that is configured on linkerd")
)

func main() {
	flag.Parse()

	fmt.Printf("Version: %s\n", version.Get())
	if *versionFlag {
		return
	}

	logrus.Infof("Using debug image %s", *debugImageFlag)
	logrus.Infof("Using cluster domain %s", *clusterDomain)

	config, err := restconfig.Default()
	if err != nil {
		logrus.Fatal(err)
	}
	config.APIPath = "api"
	config.GroupVersion = &corev1.SchemeGroupVersion
	config.NegotiatedSerializer = scheme.Codecs

	k8s := kubernetes.NewForConfigOrDie(config)

	ctx := signals.SetupSignalHandler()
	if err := controller.Start(ctx, controller.Options{
		K8s:           k8s,
		DebugImage:    *debugImageFlag,
		ClusterDomain: *clusterDomain,
	}); err != nil {
		logrus.Fatal(err)
	}
	<-ctx.Done()
	logrus.Fatal(ctx.Err())
}
