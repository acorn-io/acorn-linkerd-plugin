package controller

import (
	"testing"

	"github.com/acorn-io/acorn-linkerd-plugin/pkg/scheme"
	"github.com/acorn-io/baaah/pkg/router/tester"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestHandler_AddAnnotations(t *testing.T) {
	harness, input, err := tester.FromDir(scheme.Scheme, "testdata/annotations")
	if err != nil {
		t.Fatal(err)
	}

	req := tester.NewRequest(t, harness.Scheme, input, harness.Existing...)

	if err := AddAnnotations(req, nil); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "enabled", input.GetAnnotations()[serviceMeshAnnotation])
}

func TestHandler_KillLinkerdSidecar(t *testing.T) {
	harness, input, err := tester.FromDir(scheme.Scheme, "testdata/killsidecar")
	if err != nil {
		t.Fatal(err)
	}

	req := tester.NewRequest(t, harness.Scheme, input, harness.Existing...)

	h := Handler{
		client:     fake.NewSimpleClientset(input),
		debugImage: "foo",
	}

	if err := h.KillLinkerdSidecar(req, nil); err != nil {
		t.Fatal(err)
	}

	expected := corev1.EphemeralContainer{
		TargetContainerName: proxySidecarContainerName,
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{
			Name:            "shutdown-sidecar",
			Image:           "foo",
			ImagePullPolicy: corev1.PullAlways,
			Command: []string{
				"curl",
				"-X",
				"POST",
				"http://localhost:4191/shutdown",
			},
		},
	}
	assert.Equal(t, expected, input.(*corev1.Pod).Spec.EphemeralContainers[0])
}

func TestHandler_AddLinkerdServer(t *testing.T) {
	tester.DefaultTest(t, scheme.Scheme, "testdata/server", AddLinkerdServer)
}

func TestHandler_AddAuthorizationPolicy(t *testing.T) {
	h := Handler{
		clusterDomain:            "cluster.local",
		ingressEndpointNamespace: "kube-system",
	}
	tester.DefaultTest(t, scheme.Scheme, "testdata/authorization-policy", h.AddAuthorizationPolicy)
}

func TestHandler_AddAuthorizationPolicy_Ingress(t *testing.T) {
	h := Handler{
		clusterDomain:            "cluster.local",
		ingressEndpointNamespace: "kube-system",
	}
	tester.DefaultTest(t, scheme.Scheme, "testdata/authorization-policy-with-ingress", h.AddAuthorizationPolicy)
}

func TestHandler_NoAppNamespace(t *testing.T) {
	h := Handler{
		clusterDomain: "cluster.local",
	}
	tester.DefaultTest(t, scheme.Scheme, "testdata/no-app-namespace", h.AddAuthorizationPolicy)
}

func TestHandler_ConfigureNetworkAuthorizationForIngress(t *testing.T) {
	h := Handler{}
	tester.DefaultTest(t, scheme.Scheme, "testdata/network-authentication", h.ConfigureNetworkAuthorizationForIngress)
}

func TestHandler_ConfigureNetworkPolicyForBuildServer(t *testing.T) {
	h := Handler{
		ingressEndpointNamespace: "kube-system",
	}
	tester.DefaultTest(t, scheme.Scheme, "testdata/builder", h.ConfigureNetworkPolicyForBuildServer)
}
