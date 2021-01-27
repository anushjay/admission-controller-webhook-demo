package main

import (
	"encoding/json"
	"testing"

	admission "k8s.io/api/admission/v1beta1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	testErrAdmissionMismatch = "admission mismatch (kind: %v): got allowed=%t - wanted allowed=%t)"
)

type objectTest struct {
	testName            string
	requiredAnnotations map[string]func(string) bool
	kind                meta.GroupVersionKind
	object              interface{}
	rawObject           []byte
	ignoredNamespaces   []string
	expectedMessage     string
	shouldAllow         bool
}

func TestEnforcePodAnnotations(t *testing.T) {

	var Tests = []objectTest{
		{
			testName: "Allow Pod with required annotations",
			requiredAnnotations: map[string]func(string) bool{
				"app.kubernetes.io/name":      func(s string) bool { return true },
				"app.kubernetes.io/component": func(s string) bool { return true },
			},
			kind: meta.GroupVersionKind{
				Group:   "",
				Kind:    "Pod",
				Version: "v1",
			},
			rawObject:       []byte(`{"kind":"Pod","apiVersion":"v1","group":"","metadata":{"name":"hello-app","namespace":"default","annotations":{"app.kubernetes.io/name": "ajayaraman", "app.kubernetes.io/component": "controller"}},"spec":{"containers":[{"name":"nginx","image":"nginx:latest"}]}}`),
			expectedMessage: "",
			shouldAllow:     true,
		},
		{
			testName: "Deny Pod with required annotations",
			requiredAnnotations: map[string]func(string) bool{
				"app.kubernetes.io/name":      func(s string) bool { return true },
				"app.kubernetes.io/component": func(s string) bool { return true },
			},
			kind: meta.GroupVersionKind{
				Group:   "",
				Kind:    "Pod",
				Version: "v1",
			},
			rawObject:       []byte(`{"kind":"Pod","apiVersion":"v1","group":"","metadata":{"name":"hello-app","namespace":"default","annotations":{"app.kubernetes.io/name": "ajayaraman"}},"spec":{"containers":[{"name":"nginx","image":"nginx:latest"}]}}`),
			expectedMessage: "the submitted Pods are missing required annotations: map[app.kubernetes.io/component:key was not found]",
			shouldAllow:     false,
		},
	}

	for _, tt := range Tests {
		t.Run(tt.testName, func(t *testing.T) {
			incomingReview := admission.AdmissionReview{
				Request: &admission.AdmissionRequest{},
			}

			incomingReview.Request.Kind = tt.kind

			if tt.rawObject == nil {
				serialized, err := json.Marshal(tt.object)
				if err != nil {
					t.Fatalf("could not marshal k8s API object: %v", err)
				}

				incomingReview.Request.Object.Raw = serialized
			} else {
				incomingReview.Request.Object.Raw = tt.rawObject
			}

			resp := EnforcePodAnnotations(&incomingReview)
			if resp.Allowed != tt.shouldAllow {
				t.Fatalf(testErrAdmissionMismatch, tt.kind, resp.Allowed, tt.shouldAllow)
			}
		})
	}

}
