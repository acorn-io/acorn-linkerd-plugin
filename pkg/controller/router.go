package controller

import (
	"github.com/acorn-io/baaah/pkg/router"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
)

var (
	projectSelector = labels.SelectorFromSet(map[string]string{
		"acorn.io/project": "true",
	})

	acornManagedSelector = labels.SelectorFromSet(map[string]string{
		"acorn.io/managed": "true",
	})

	appNameLabel      = "acorn.io/app-name"
	appNamespaceLabel = "acorn.io/app-namespace"
	jobLabel          = "acorn.io/job-name"

	acornImageSystemNamespace = "acorn-image-system"
)

func RegisterRoutes(router *router.Router, client kubernetes.Interface, debugImage, clusterDomain, ingressEndpointName, ingressEndpointNamespace string) error {
	h := Handler{
		client:                   client,
		debugImage:               debugImage,
		clusterDomain:            clusterDomain,
		ingressEndpointName:      ingressEndpointName,
		ingressEndpointNamespace: ingressEndpointNamespace,
	}

	managedSelector, err := getAcornManagedSelector()
	if err != nil {
		return err
	}

	jobSelector, err := getJobPodSelector()
	if err != nil {
		return err
	}

	router.Type(&corev1.Namespace{}).Selector(projectSelector).HandlerFunc(AddAnnotations)
	router.Type(&corev1.Pod{}).Selector(managedSelector).Selector(jobSelector).HandlerFunc(h.KillLinkerdSidecar)
	router.Type(&corev1.Endpoints{}).Namespace(h.ingressEndpointNamespace).Name(h.ingressEndpointName).HandlerFunc(h.ConfigureNetworkAuthorizationForIngress)
	router.Type(&corev1.Service{}).Selector(managedSelector).HandlerFunc(AddLinkerdServer)
	router.Type(&corev1.Namespace{}).Selector(projectSelector).HandlerFunc(h.AddAuthorizationPolicy)
	router.Type(&appsv1.Deployment{}).Namespace(acornImageSystemNamespace).HandlerFunc(h.ConfigureNetworkPolicyForBuildServer)

	return nil
}

func getAcornManagedSelector() (labels.Selector, error) {
	r1, err := labels.NewRequirement(appNameLabel, selection.Exists, nil)
	if err != nil {
		return nil, err
	}
	r2, err := labels.NewRequirement(appNamespaceLabel, selection.Exists, nil)
	if err != nil {
		return nil, err
	}
	acornManagedSelector.Add(*r1, *r2)
	return acornManagedSelector, nil
}

func getJobPodSelector() (labels.Selector, error) {
	r1, err := labels.NewRequirement(jobLabel, selection.Exists, nil)
	if err != nil {
		return nil, err
	}
	return labels.NewSelector().Add(*r1), nil
}
