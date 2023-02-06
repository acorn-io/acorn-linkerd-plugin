package controller

import (
	"fmt"
	"sort"
	"strings"

	"github.com/acorn-io/baaah/pkg/name"
	"github.com/acorn-io/baaah/pkg/router"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
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

	ingressNetworkAuthenticationName = "acorn-ingress-network-authentication"
	serviceNameLabel                 = "acorn.io/service-name"
)

type Handler struct {
	client                   kubernetes.Interface
	debugImage               string
	clusterDomain            string
	ingressEndpointName      string
	ingressEndpointNamespace string
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
			Name:            "shutdown-sidecar",
			Image:           h.debugImage,
			ImagePullPolicy: corev1.PullAlways,
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

	if service.Spec.Selector == nil {
		return nil
	}

	for _, port := range service.Spec.Ports {
		resp.Objects(&serverv1beta1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: service.Namespace,
				// We always program service port name in acorn
				Name: fmt.Sprintf("%v-%v", service.Name, port.Name),
				Labels: map[string]string{
					serviceNameLabel: service.Name,
				},
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
	sort.SliceStable(appNamespaces.Items, func(i, j int) bool {
		return appNamespaces.Items[i].Name < appNamespaces.Items[j].Name
	})
	for _, appNamespace := range appNamespaces.Items {
		serviceaccountsIdentities = append(serviceaccountsIdentities, fmt.Sprintf("*.%s.serviceaccount.identity.linkerd.%v", appNamespace.Name, h.clusterDomain))
	}

	if len(serviceaccountsIdentities) == 0 {
		return nil
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
	ingressNamespace := gatewayapiv1alpha2.Namespace(h.ingressEndpointNamespace)

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

		// Check if service is referenced by an ingress, and if so, create an authorization policy that
		// allow traffic from ingress pod
		// Todo: For now we want to allow access from ingress by default. We can program some smart way to figure out whether service needs to be exposed by ingress

		resp.Objects(&policyv1alpha1.AuthorizationPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: server.Namespace,
				Name:      name.SafeConcatName("authz-profile-ingress", server.Name),
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
						Kind:      "NetworkAuthentication",
						Name:      ingressNetworkAuthenticationName,
						Namespace: &ingressNamespace,
					},
				},
			},
		})
	}

	return nil
}

// ConfigureNetworkAuthorizationForIngress configures the authorization policy so that
// Ingress pod is able to reach acorn apps. This should normally be done through service
// account identity but not sure why it is not working.
// TODO: need to figure out how service account works when ingress mode is enabled
func (h Handler) ConfigureNetworkAuthorizationForIngress(req router.Request, resp router.Response) error {
	ingressEndpoint := req.Object.(*corev1.Endpoints)

	var networks []*policyv1alpha1.Network
	for _, subnet := range ingressEndpoint.Subsets {
		for _, address := range subnet.Addresses {
			networks = append(networks, &policyv1alpha1.Network{
				Cidr: address.IP,
			})
		}
	}

	resp.Objects(&policyv1alpha1.NetworkAuthentication{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ingressEndpoint.Namespace,
			Name:      ingressNetworkAuthenticationName,
		},
		Spec: policyv1alpha1.NetworkAuthenticationSpec{
			Networks: networks,
		},
	})

	return nil
}

// ConfigureNetworkPolicyForBuildServer configures network policy for buildkit servers so that they can't talk to each other
func (h Handler) ConfigureNetworkPolicyForBuildServer(req router.Request, resp router.Response) error {
	builderDeployment := req.Object.(*appsv1.Deployment)

	// we want to skip any service that is not starting with "bld".
	// Since when --builder-per-project is enabled, the deployment name always starts with bld
	if !strings.HasPrefix(builderDeployment.Name, "bld") {
		return nil
	}

	if builderDeployment.Spec.Template.Annotations[serviceMeshAnnotation] != "enabled" {
		if builderDeployment.Spec.Template.Annotations == nil {
			builderDeployment.Spec.Template.Annotations = map[string]string{}
		}
		builderDeployment.Spec.Template.Annotations[serviceMeshAnnotation] = "enabled"
		if err := req.Client.Update(req.Ctx, builderDeployment); err != nil {
			return err
		}
	}

	var builderService corev1.Service
	if err := req.Client.Get(req.Ctx, client.ObjectKey{
		Namespace: builderDeployment.Namespace,
		Name:      builderDeployment.Name,
	}, &builderService); err != nil {
		return err
	}

	ingressNamespace := gatewayapiv1alpha2.Namespace(h.ingressEndpointNamespace)

	for _, port := range builderService.Spec.Ports {
		server := &serverv1beta1.Server{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: builderService.Namespace,
				// We always program service port name in acorn
				Name: fmt.Sprintf("%v-%v", builderService.Name, port.Name),
				Labels: map[string]string{
					serviceNameLabel: builderService.Name,
				},
			},
			Spec: serverv1beta1.ServerSpec{
				PodSelector: metav1.SetAsLabelSelector(builderService.Spec.Selector),
				Port:        intstr.FromInt(int(port.Port)),
			},
		}
		resp.Objects(server)

		resp.Objects(&policyv1alpha1.AuthorizationPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: server.Namespace,
				Name:      name.SafeConcatName("authz-profile-ingress", server.Name),
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
						Kind:      "NetworkAuthentication",
						Name:      ingressNetworkAuthenticationName,
						Namespace: &ingressNamespace,
					},
				},
			},
		})
	}

	return nil
}
