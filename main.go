package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

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
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			kubeconfig = os.Getenv("HOME") + "/.kube/config"
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			slog.Error("Failed to build config", "error", err)
			os.Exit(1)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		slog.Error("Failed to create clientset", "error", err)
		os.Exit(1)
	}

	listWatch := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "events", metav1.NamespaceAll, fields.Everything())
	informer := cache.NewSharedInformer(listWatch, &corev1.Event{}, 0)

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			event := obj.(*corev1.Event)
			handleEvent(event, clientset)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			event := newObj.(*corev1.Event)
			handleEvent(event, clientset)
		},
	})

	stopCh := make(chan struct{})
	defer close(stopCh)
	go informer.Run(stopCh)
	<-stopCh
}

func handleEvent(event *corev1.Event, clientset *kubernetes.Clientset) {
	const scaleUp = "KEDAScaleTargetActivated"
	const scaleDown = "KEDAScaleTargetDeactivated"

	if event.Source.Component != "keda-operator" {
		return
	}

	name := event.InvolvedObject.Name
	if name != "dhis2-core" {
		return
	}

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

	postgresName := baseName + "-postgresql"
	scaleStatefulSets(clientset, namespace, postgresName, scaleTo)
	//minioName := baseName + "-minio"
	//scaleDeployment(clientset, namespace, minioName, scaleTo)
}

func scaleStatefulSets(clientset *kubernetes.Clientset, namespace, name string, replicas int32) {
	slog.Info("Scale", "name", name, "replicas", replicas)

	patch := fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas)
	_, err := clientset.AppsV1().StatefulSets(namespace).Patch(context.TODO(), name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	if err != nil {
		slog.Error("Failed to scale deployment", "name", name, "error", err)
	}
}
