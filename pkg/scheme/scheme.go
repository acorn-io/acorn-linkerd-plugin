package scheme

import (
	policyv1alpha1 "github.com/linkerd/linkerd2/controller/gen/apis/policy/v1alpha1"
	serverv1beta1 "github.com/linkerd/linkerd2/controller/gen/apis/server/v1beta1"
	"github.com/rancher/wrangler/pkg/merr"
	appsv1 "k8s.io/api/apps/v1"
	authv1 "k8s.io/api/authorization/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

var (
	Scheme         = runtime.NewScheme()
	Codecs         = serializer.NewCodecFactory(Scheme)
	ParameterCodec = runtime.NewParameterCodec(Scheme)
)

func AddToScheme(scheme *runtime.Scheme) error {
	var errs []error
	errs = append(errs, corev1.AddToScheme(scheme))
	errs = append(errs, appsv1.AddToScheme(scheme))
	errs = append(errs, batchv1.AddToScheme(scheme))
	errs = append(errs, networkingv1.AddToScheme(scheme))
	errs = append(errs, storagev1.AddToScheme(scheme))
	errs = append(errs, rbacv1.AddToScheme(scheme))
	errs = append(errs, authv1.AddToScheme(scheme))
	errs = append(errs, apiextensionv1.AddToScheme(scheme))
	errs = append(errs, serverv1beta1.AddToScheme(scheme))
	errs = append(errs, policyv1alpha1.AddToScheme(scheme))
	return merr.NewErrors(errs...)
}

func init() {
	utilruntime.Must(AddToScheme(Scheme))
}
