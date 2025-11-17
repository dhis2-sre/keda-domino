package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func run() error {
	targetNamespaces := getEnvAsSlice("TARGET_NAMESPACES")
	if len(targetNamespaces) == 0 {
		return fmt.Errorf("TARGET_NAMESPACES env var is required")
	}

	clientset, err := newClientset()
	if err != nil {
		return err
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	synced := false
	informers := make([]cache.SharedInformer, 0, len(targetNamespaces))

	for _, namespace := range targetNamespaces {
		listWatch := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "events", namespace, fields.Everything())
		informer := cache.NewSharedInformer(listWatch, &corev1.Event{}, 0)

		_, err = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				if !synced {
					return
				}
				event := obj.(*corev1.Event)
				handleEvent(event, clientset)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				event := newObj.(*corev1.Event)
				handleEvent(event, clientset)
			},
		})
		if err != nil {
			return err
		}

		informers = append(informers, informer)
		go informer.Run(stopCh)
	}

	for i, informer := range informers {
		if !cache.WaitForCacheSync(stopCh, informer.HasSynced) {
			return fmt.Errorf("failed to sync informer cache for namespace %s", targetNamespaces[i])
		}
	}
	synced = true

	slog.Info("Informers synced, watching for new events", "targetNamespaces", targetNamespaces)

	healthCheck()

	<-stopCh
	return nil
}

func newClientset() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			return nil, fmt.Errorf("no kubeconfig found! Make sure $KUBECONFIG points to a valid kubeconfig file")
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}

func handleEvent(event *corev1.Event, clientset *kubernetes.Clientset) {
	const scaleUp = "KEDAScaleTargetActivated"
	const scaleDown = "KEDAScaleTargetDeactivated"

	if event.Source.Component != "keda-operator" {
		return
	}

	name := event.InvolvedObject.Name
	var scaleTo int32
	switch event.Reason {
	case scaleUp:
		scaleTo = 1
		slog.Info("Scaled up", "name", name)
	case scaleDown:
		scaleTo = 0
		slog.Info("Scaled down", "name", name)
	default:
		return
	}

	baseName := strings.TrimSuffix(name, "-core")

	namespace := event.InvolvedObject.Namespace
	postgresName := baseName + "-database-postgresql"
	scaleStatefulSets(clientset, namespace, postgresName, scaleTo)
	// minioName := baseName + "-minio"
	// scaleDeployment(clientset, namespace, minioName, scaleTo)
}

func scaleStatefulSets(clientset *kubernetes.Clientset, namespace, name string, replicas int32) {
	slog.Info("Scale", "name", name, "replicas", replicas)

	patch := fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas)
	_, err := clientset.AppsV1().StatefulSets(namespace).Patch(context.TODO(), name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	if err != nil {
		slog.Error("Failed to scale deployment", "name", name, "error", err)
	}
}

func healthCheck() {
	http.HandleFunc("/health", healthHandler)

	server := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil {
			slog.Error("HTTP server error", "error", err)
		}
	}()
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func getEnvAsSlice(envVar string) []string {
	value := os.Getenv(envVar)
	if value == "" {
		return []string{}
	}

	return strings.Split(value, ",")
}
