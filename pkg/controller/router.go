package controller

import (
	"github.com/acorn-io/baaah/pkg/router"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

var (
	projectSelector = labels.SelectorFromSet(map[string]string{
		"acorn.io/project": "true",
	})

	acornManagedSelector = labels.SelectorFromSet(map[string]string{
		"acorn.io/managed": "true",
	})
)

func RegisterRoutes(router *router.Router, client *kubernetes.Clientset) {
	router.Type(&corev1.Namespace{}).Selector(projectSelector).HandlerFunc(AddAnnotations)
	router.Type(&corev1.Namespace{}).Selector(projectSelector).HandlerFunc(AddAuthorizationPolicy)

	r := Reaper{
		client: client,
	}
	router.Type(&corev1.Pod{}).Selector(acornManagedSelector).HandlerFunc(r.KillLinkerdSidecar)

	router.Type(&corev1.Service{}).Selector(acornManagedSelector).HandlerFunc(AddLinkerdServer)

}
