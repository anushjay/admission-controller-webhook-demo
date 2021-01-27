package main

import (
	"fmt"
	"k8s.io/api/admission/v1beta1"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"log"
)

// We check whether the (strictly matched) annotation key exists, and then run
// our user-provided matchFunc against it. If we're missing any keys, or the
// value for a key does not match, admission is rejected.
var (
	requiredAnnotations = map[string]func(string) bool{
		"app.kubernetes.io/name":      func(string) bool { return true },
		"app.kubernetes.io/component": func(string) bool { return true },
	}
)

var (
	decodeFailure        = "unable to decode request from server"
	podDeniedError       = "the submitted Pods are missing required annotations"
	unsupportedKindError = "the submitted Kind is not supported by this admission handler"
)

var (
	universalDeserializer = serializer.NewCodecFactory(runtime.NewScheme()).UniversalDeserializer()
)

// isKubeNamespace checks if the given namespace is a Kubernetes-owned namespace.
func isKubeNamespace(ns string) bool {
	return ns == v1.NamespacePublic || ns == v1.NamespaceSystem
}

// Default response
func newDefaultDenyResponse() *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{
		Allowed: false,
		Result: &v1.Status{
			Status:  "Denied",
			Message: "Unknown",
		},
	}
}

// EnforcePodAnnotations implements the logic of our example admission controller webhook.
func EnforcePodAnnotations(review *v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	// Extract the necessary metadata from our known Kinds
	kind := review.Request.Kind.Kind
	ns := review.Request.Namespace
	log.Println(fmt.Sprintf("Enforcing annotations for %s in ns %s", kind, ns))
	resp := newDefaultDenyResponse()

	var namespace string
	annotations := make(map[string]string)

	// Ignore checks in kube system and public namespaces
	if isKubeNamespace(ns) {
		resp.Allowed = true
		resp.Result.Message = fmt.Sprintf("allowing admission: %s namespace is whitelisted", namespace)
		return resp
	}

	switch kind {
	case "Pod":
		pod := core.Pod{}
		if _, _, err := universalDeserializer.Decode(review.Request.Object.Raw, nil, &pod); err != nil {
			resp.Result.Message = decodeFailure
			return resp
		}

		namespace = pod.GetNamespace()
		annotations = pod.GetAnnotations()
	case "Deployment":
		deployment := apps.Deployment{}
		if _, _, err := universalDeserializer.Decode(review.Request.Object.Raw, nil, &deployment); err != nil {
			resp.Result.Message = decodeFailure
			return resp
		}

		deployment.GetNamespace()
		annotations = deployment.Spec.Template.GetAnnotations()
	case "ReplicaSet":
		replicaSet := apps.ReplicaSet{}
		if _, _, err := universalDeserializer.Decode(review.Request.Object.Raw, nil, &replicaSet); err != nil {
			resp.Result.Message = decodeFailure
			return resp
		}

		namespace = replicaSet.GetNamespace()
		annotations = replicaSet.Spec.Template.GetAnnotations()
	case "StatefulSet":
		statefulset := apps.StatefulSet{}
		if _, _, err := universalDeserializer.Decode(review.Request.Object.Raw, nil, &statefulset); err != nil {
			resp.Result.Message = decodeFailure
			return resp
		}

		namespace = statefulset.GetNamespace()
		annotations = statefulset.Spec.Template.GetAnnotations()
	case "DaemonSet":
		daemonset := apps.DaemonSet{}
		if _, _, err := universalDeserializer.Decode(review.Request.Object.Raw, nil, &daemonset); err != nil {
			resp.Result.Message = decodeFailure
			return resp
		}

		namespace = daemonset.GetNamespace()
		annotations = daemonset.Spec.Template.GetAnnotations()
	default:
		resp.Result.Message = unsupportedKindError
		return resp
	}

	missing := make(map[string]string)
	for requiredKey, matchFunc := range requiredAnnotations {
		if matchFunc == nil {
			resp.Result.Message = fmt.Sprintf("cannot validate annotations (%s) with a nil matchFunc", requiredKey)
			return resp
		}

		if existingVal, ok := annotations[requiredKey]; !ok {
			// Key does not exist; add it to the missing annotations list
			missing[requiredKey] = "key was not found"
		} else {
			if matched := matchFunc(existingVal); !matched {
				missing[requiredKey] = "value did not match"
			}
			// Key exists & matchFunc returned OK.
		}
	}

	if len(missing) > 0 {
		resp.Result.Message = fmt.Sprintf("%s: %v", podDeniedError, missing)
		return resp
	}

	// No missing or invalid annotations; allow admission
	resp.Allowed = true
	return resp
}
