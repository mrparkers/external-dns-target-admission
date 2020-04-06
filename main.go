package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	v1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

type controller struct {
	port      int
	tlsSecret string
	ipAddress string
}

type jsonStringPatchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value string `json:"value"`
}

type jsonMapPatchOperation struct {
	Op    string            `json:"op"`
	Path  string            `json:"path"`
	Value map[string]string `json:"value"`
}

type objectWithMeta struct {
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
}

func main() {
	var controller controller

	flag.IntVar(&controller.port, "port", 8080, "The port the mutating admission webhook will listen on")
	flag.StringVar(&controller.tlsSecret, "tlsSecret", "", "The Kubernetes secret containing the tls.crt and tls.key")
	flag.StringVar(&controller.ipAddress, "ipAddress", "", "The IP address that each Ingress and Gateway should be annotated with")

	flag.Parse()

	if controller.tlsSecret == "" {
		log.Fatal("tlsSecret command line flag must be specified")
	}

	if controller.ipAddress == "" {
		log.Fatal("ipAddress command line flag must be specified")
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatal(err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal(err)
	}

	namespace, err := getCurrentNamespace()
	if err != nil {
		log.WithField("error", err).Fatal("error retrieving controller namespace")
	}

	secret, err := client.CoreV1().Secrets(namespace).Get(controller.tlsSecret, metav1.GetOptions{})
	if err != nil {
		log.Fatal(err)
	}

	certData, ok := secret.Data["tls.crt"]
	if !ok {
		log.Fatalf("secret %s missing tls.crt", controller.tlsSecret)
	}

	keyData, ok := secret.Data["tls.key"]
	if !ok {
		log.Fatalf("secret %s missing tls.key", controller.tlsSecret)
	}

	certFile, err := ioutil.TempFile(os.TempDir(), "cert-")
	if err != nil {
		log.Fatal(err)
	}

	keyFile, err := ioutil.TempFile(os.TempDir(), "key-")
	if err != nil {
		log.Fatal(err)
	}

	defer os.Remove(certFile.Name())
	defer os.Remove(keyFile.Name())

	if _, err := certFile.Write(certData); err != nil {
		log.Fatal(err)
	}

	if err := certFile.Close(); err != nil {
		log.Fatal(err)
	}

	if _, err := keyFile.Write(keyData); err != nil {
		log.Fatal(err)
	}

	if err := keyFile.Close(); err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", controller.webhook)
	mux.HandleFunc("/healthz", healthz)

	server := http.Server{
		Addr:    fmt.Sprintf(":%d", controller.port),
		Handler: mux,
	}

	go func() {
		err := server.ListenAndServeTLS(certFile.Name(), keyFile.Name())
		if err != nil {
			log.Fatal(err)
		}
	}()

	log.Info("Listening...")

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM)

	<-s

	log.Info("Shutting down...")
	server.Shutdown(context.Background())
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func getCurrentNamespace() (string, error) {
	b, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(b)), nil
}

func (controller *controller) webhook(w http.ResponseWriter, r *http.Request) {
	log.Info("received request")

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.WithField("error", err).Errorf("can't read request body")
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	defer r.Body.Close()

	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		log.WithField("content type", contentType).Error("unexpected content type")
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	admissionReview := v1.AdmissionReview{}
	err = json.Unmarshal(body, &admissionReview)
	if err != nil {
		log.WithField("error", err).Errorf("unable to unmarshal admission review")
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	admissionReview.Response = controller.mutate(&admissionReview)
	response, err := json.Marshal(admissionReview)
	if err != nil {
		log.WithField("error", err).Errorf("unable to marshal response")
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	_, err = w.Write(response)
	if err != nil {
		log.WithField("error", err).Errorf("error sending response")
		w.WriteHeader(http.StatusInternalServerError)

		return
	}
}

func (controller *controller) mutate(admissionReview *v1.AdmissionReview) *v1.AdmissionResponse {
	log.WithFields(log.Fields{
		"kind":      admissionReview.Request.Kind,
		"name":      admissionReview.Request.Name,
		"namespace": admissionReview.Request.Namespace,
		"operation": admissionReview.Request.Operation,
	}).Info("admission review")

	objectKind := admissionReview.Request.Kind.Kind
	if objectKind != "Ingress" && objectKind != "Gateway" {
		log.WithField("kind", objectKind).Info("not adding annotation to object")

		return &v1.AdmissionResponse{
			Allowed: true,
		}
	}

	var objectMeta objectWithMeta
	if err := json.Unmarshal(admissionReview.Request.Object.Raw, &objectMeta); err != nil {
		log.WithField("error", err).Error("error unmarshaling ingress")
		return &v1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	if ip, exists := objectMeta.Annotations["external-dns.alpha.kubernetes.io/target"]; exists {
		log.WithField("ip", ip).Info("not mutating object that already has annotation")
		return &v1.AdmissionResponse{
			Allowed: true,
		}
	}

	patchBytes, err := controller.getJsonPatch(objectMeta)
	if err != nil {
		return &v1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	patchType := v1.PatchTypeJSONPatch
	return &v1.AdmissionResponse{
		Allowed:   true,
		Patch:     patchBytes,
		PatchType: &patchType,
		UID:       admissionReview.Request.UID,
	}
}

func (controller *controller) getJsonPatch(meta objectWithMeta) ([]byte, error) {
	if len(meta.Annotations) == 0 {
		mapPatch := []jsonMapPatchOperation{
			{
				Op:   "add",
				Path: "/metadata/annotations",
				Value: map[string]string{
					"external-dns.alpha.kubernetes.io/target": controller.ipAddress,
				},
			},
		}
		return json.Marshal(mapPatch)
	}

	stringPatch := []jsonStringPatchOperation{
		{
			Op:    "add",
			Path:  fmt.Sprintf("/metadata/annotations/%s", "external-dns.alpha.kubernetes.io~1target"),
			Value: controller.ipAddress,
		},
	}
	return json.Marshal(stringPatch)
}
