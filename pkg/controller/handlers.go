package controller

import (
	"fmt"

	"github.com/acorn-io/baaah/pkg/name"
	"github.com/acorn-io/baaah/pkg/router"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapiv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	policyv1alpha1 "github.com/linkerd/linkerd2/controller/gen/apis/policy/v1alpha1"
	serverv1beta1 "github.com/linkerd/linkerd2/controller/gen/apis/server/v1beta1"
)

const (
	serviceMeshAnnotation     = "linkerd.io/inject"
	proxySidecarContainerName = "linkerd-proxy"
)

type Handler struct {
	client        kubernetes.Interface
	debugImage    string
	clusterDomain string
}

// AddAnnotations adds linkerd annotations to all acorn projects so that it can propagate into app namespaces
func AddAnnotations(req router.Request, resp router.Response) error {
	projectNamespace := req.Object.(*corev1.Namespace)

	if projectNamespace.Annotations == nil {
		projectNamespace.Annotations = map[string]string{}
	}

	if projectNamespace.Annotations[serviceMeshAnnotation] == "enabled" {
		return nil
	}

	logrus.Infof("Updating project %v to inject linkerd service mesh annotation", projectNamespace.Name)
	projectNamespace.Annotations[serviceMeshAnnotation] = "enabled"
	if err := req.Client.Update(req.Ctx, projectNamespace); err != nil {
		return err
	}
	return nil
}

// KillLinkerdSidecar finds all the pods that belongs to acorn jobs but stuck at completing because of linkerd sidecar. It launches ephemeral container to kill sidecar
func (h Handler) KillLinkerdSidecar(req router.Request, resp router.Response) error {
	pod := req.Object.(*corev1.Pod)

	// we want to ignore all the pods that doesn't belong to acorn jobs
	if _, ok := pod.Labels["acorn.io/job-name"]; !ok {
		return nil
	}

	// wait for all the containers to terminate
	foundSidecar := false
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.Name != proxySidecarContainerName && containerStatus.State.Terminated == nil {
			return nil
		}

		if containerStatus.Name == proxySidecarContainerName {
			foundSidecar = true
		}
	}

	if !foundSidecar {
		return nil
	}

	// If pod is already configured with ephemeral container, skip
	if len(pod.Spec.EphemeralContainers) > 0 {
		return nil
	}

	logrus.Infof("Launching ephemeral container to kill pod %v/%v sidecar", pod.Namespace, pod.Name)
	pod.Spec.EphemeralContainers = append(pod.Spec.EphemeralContainers, corev1.EphemeralContainer{
		TargetContainerName: proxySidecarContainerName,
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{
			Name:  "shutdown-sidecar",
			Image: h.debugImage,
			Command: []string{
				"curl",
				"-X",
				"POST",
				"http://localhost:4191/shutdown",
			},
		},
	})
	if _, err := h.client.CoreV1().Pods(pod.Namespace).UpdateEphemeralContainers(req.Ctx, pod.Name, pod, metav1.UpdateOptions{}); err != nil {
		return err
	}

	return nil
}

// AddLinkerdServer adds linkerd server CRD to each acorn apps. This will create a policy to disallow apps from
// talking to each other unless a specific AuthorizationPolicy is defined.
func AddLinkerdServer(req router.Request, resp router.Response) error {
	service := req.Object.(*corev1.Service)

	for _, port := range service.Spec.Ports {
		resp.Objects(&serverv1beta1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: service.Namespace,
				// We always program service port name in acorn
				Name: fmt.Sprintf("%v-%v", service.Name, port.Name),
			},
			Spec: serverv1beta1.ServerSpec{
				PodSelector: metav1.SetAsLabelSelector(service.Spec.Selector),
				Port:        intstr.FromInt(int(port.Port)),
			},
		})
	}
	return nil
}

/*
AddAuthorizationPolicy makes sure within each acorn project, apps can talk to each other. It does the following:
1. Programs MeshTLSAuthentication for each app namespaces to represent all the service account identities in the same project
2. For each server, create an AuthorizationPolicy per project to allow network access.
*/
func (h Handler) AddAuthorizationPolicy(req router.Request, resp router.Response) error {
	projectNamespace := req.Object.(*corev1.Namespace)

	var appNamespaces corev1.NamespaceList
	if err := req.Client.List(req.Ctx, &appNamespaces, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"acorn.io/app-namespace": projectNamespace.Name,
		}),
	}); err != nil {
		return err
	}

	// First, we create a MeshTLSAuthentication representing all the service accounts in the current project
	var serviceaccountsIdentities []string
	for _, appNamespace := range appNamespaces.Items {
		serviceaccountsIdentities = append(serviceaccountsIdentities, fmt.Sprintf("*.%s.serviceaccount.identity.linkerd.%v", appNamespace.Name, h.clusterDomain))
	}
	resp.Objects(&policyv1alpha1.MeshTLSAuthentication{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: projectNamespace.Name,
			Name:      name.SafeConcatName("mesh-authn-profile", projectNamespace.Name),
		},
		Spec: policyv1alpha1.MeshTLSAuthenticationSpec{
			Identities: serviceaccountsIdentities,
		},
	})

	// Second, For each Server(k8s service), we create an AuthorizationPolicy to allow network access to
	// from all service account identities from the same project
	var servers serverv1beta1.ServerList
	for _, ns := range appNamespaces.Items {
		var result serverv1beta1.ServerList
		if err := req.Client.List(req.Ctx, &result, &client.ListOptions{
			Namespace: ns.Name,
		}); err != nil {
			return err
		}
		servers.Items = append(servers.Items, result.Items...)
	}
	project := gatewayapiv1alpha2.Namespace(projectNamespace.Name)
	for _, server := range servers.Items {
		resp.Objects(&policyv1alpha1.AuthorizationPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: server.Namespace,
				Name:      name.SafeConcatName("authz-profile", projectNamespace.Name, server.Name),
			},
			Spec: policyv1alpha1.AuthorizationPolicySpec{
				TargetRef: gatewayapiv1alpha2.PolicyTargetReference{
					Group: gatewayapiv1alpha2.Group(policyv1alpha1.SchemeGroupVersion.Group),
					Kind:  "Server",
					Name:  gatewayapiv1alpha2.ObjectName(server.Name),
				},
				RequiredAuthenticationRefs: []gatewayapiv1alpha2.PolicyTargetReference{
					{
						Group:     gatewayapiv1alpha2.Group(policyv1alpha1.SchemeGroupVersion.Group),
						Kind:      "MeshTLSAuthentication",
						Name:      gatewayapiv1alpha2.ObjectName(name.SafeConcatName("mesh-authn-profile", projectNamespace.Name)),
						Namespace: &project,
					},
				},
			},
		})
	}

	return nil
}
