package main

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	istioClientNetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1beta1 "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"
	"testing"
)

var c *controller
var expectedIp string

func init() {
	expectedIp = rand.String(10)
	c = &controller{
		ipAddress: expectedIp,
	}
}

func TestAnnotateIngress(t *testing.T) {
	ingress := networkingv1beta1.Ingress{}
	ingressBytes, err := json.Marshal(ingress)
	if err != nil {
		t.Fatal(err)
	}

	admissionReview := &admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{},
		Request: &admissionv1.AdmissionRequest{
			Kind: metav1.GroupVersionKind{
				Kind: "Ingress",
			},
			Object: runtime.RawExtension{
				Raw: ingressBytes,
			},
		},
	}

	admissionResponse := c.mutate(admissionReview)

	assert.True(t, admissionResponse.Allowed)
	assert.Equal(t, admissionv1.PatchTypeJSONPatch, *admissionResponse.PatchType)

	var patch []jsonPatchOperation
	err = json.Unmarshal(admissionResponse.Patch, &patch)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "add", patch[0].Op, `expect patch operation to be "add""`)
	assert.Equal(t, "/metadata/annotations/external-dns.alpha.kubernetes.io~1target", patch[0].Path)
	assert.Equal(t, expectedIp, patch[0].Value)
}

func TestAnnotateGateway(t *testing.T) {
	gateway := istioClientNetworkingv1beta1.Gateway{}
	gatewayBytes, err := json.Marshal(gateway)
	if err != nil {
		t.Fatal(err)
	}

	admissionReview := &admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{},
		Request: &admissionv1.AdmissionRequest{
			Kind: metav1.GroupVersionKind{
				Kind: "Gateway",
			},
			Object: runtime.RawExtension{
				Raw: gatewayBytes,
			},
		},
	}

	admissionResponse := c.mutate(admissionReview)

	assert.True(t, admissionResponse.Allowed)
	assert.Equal(t, admissionv1.PatchTypeJSONPatch, *admissionResponse.PatchType)

	var patch []jsonPatchOperation
	err = json.Unmarshal(admissionResponse.Patch, &patch)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "add", patch[0].Op, `expect patch operation to be "add""`)
	assert.Equal(t, "/metadata/annotations/external-dns.alpha.kubernetes.io~1target", patch[0].Path)
	assert.Equal(t, expectedIp, patch[0].Value)
}

func TestDontAnnotatePod(t *testing.T) {
	pod := corev1.Pod{}
	podBytes, err := json.Marshal(pod)
	if err != nil {
		t.Fatal(err)
	}

	admissionReview := &admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{},
		Request: &admissionv1.AdmissionRequest{
			Kind: metav1.GroupVersionKind{
				Kind: "Pod",
			},
			Object: runtime.RawExtension{
				Raw: podBytes,
			},
		},
	}

	admissionResponse := c.mutate(admissionReview)

	assert.True(t, admissionResponse.Allowed)
	assert.Nil(t, admissionResponse.PatchType)
	assert.Empty(t, admissionResponse.Patch)
}

func TestDontAnnotateIngressWithExistingTarget(t *testing.T) {
	ingress := networkingv1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"external-dns.alpha.kubernetes.io/target": "foo",
			},
		},
	}
	ingressBytes, err := json.Marshal(ingress)
	if err != nil {
		t.Fatal(err)
	}

	admissionReview := &admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{},
		Request: &admissionv1.AdmissionRequest{
			Kind: metav1.GroupVersionKind{
				Kind: "Ingress",
			},
			Object: runtime.RawExtension{
				Raw: ingressBytes,
			},
		},
	}

	admissionResponse := c.mutate(admissionReview)

	assert.True(t, admissionResponse.Allowed)
	assert.Nil(t, admissionResponse.PatchType)
	assert.Empty(t, admissionResponse.Patch)
}
