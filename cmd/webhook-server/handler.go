package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	admission "k8s.io/api/admission/v1beta1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"log"
	"net/http"
)

const (
	jsonContentType = `application/json`
)

// admitFunc is a callback for admission controller logic. Given an AdmissionRequest, it returns the sequence of patch
// operations to be applied in case of success, or the error that will be shown when the operation is rejected.
type admitFunc func(review *admission.AdmissionReview) *admission.AdmissionResponse

// doServeAdmitFunc parses the HTTP request for an admission controller webhook, and -- in case of a well-formed
// request -- delegates the admission control logic to the given admitFunc. The response body is then returned as raw
// bytes.
func doServeAdmitFunc(w http.ResponseWriter, r *http.Request, admit admitFunc) ([]byte, error) {
	// Request validation. Only handle POST requests with a body and json content type.
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return nil, fmt.Errorf("invalid method %s, only POST requests are allowed", r.Method)
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return nil, fmt.Errorf("could not read request body: %v", err)
	}

	if contentType := r.Header.Get("Content-Type"); contentType != jsonContentType {
		w.WriteHeader(http.StatusBadRequest)
		return nil, fmt.Errorf("unsupported content type %s, only %s is supported", contentType, jsonContentType)
	}

	// Set HTTP headers
	w.Header().Set("Content-Type", jsonContentType)

	// Parse the AdmissionReview request.
	incomingReview := admission.AdmissionReview{}
	if _, _, err := universalDeserializer.Decode(body, nil, &incomingReview); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return nil, fmt.Errorf("could not deserialize request: %v", err)
	} else if incomingReview.Request == nil {
		w.WriteHeader(http.StatusBadRequest)
		return nil, errors.New("malformed admission review: request is nil")
	}

	// Run admit functions
	admissionReviewResponse := admit(&incomingReview)

	// Construct outgoing admission review response
	outgoingReview := &admission.AdmissionReview{
		Response: &admission.AdmissionResponse{
			UID:     incomingReview.Request.UID,
			Allowed: admissionReviewResponse.Allowed,
			Result: &meta.Status{
				Message: admissionReviewResponse.Result.Message,
			},
		},
	}

	// Return the AdmissionReview with a response as JSON.
	bytes, err := json.Marshal(&outgoingReview)
	if err != nil {
		return nil, fmt.Errorf("marshaling response: %v", err)
	}
	return bytes, nil
}

// serveAdmitFunc is a wrapper around doServeAdmitFunc that adds error handling and logging.
func serveAdmitFunc(w http.ResponseWriter, r *http.Request, admit admitFunc) {
	log.Print("Handling webhook request ...")
	var writeErr error
	if bytes, err := doServeAdmitFunc(w, r, admit); err != nil {
		log.Printf("Error handling webhook request: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, writeErr = w.Write([]byte(err.Error()))
	} else {
		log.Print("Webhook request handled successfully")
		w.WriteHeader(http.StatusOK)
		_, writeErr = w.Write(bytes)
	}

	if writeErr != nil {
		log.Printf("Could not write response: %v", writeErr)
	}
}

// admitFuncHandler takes an admitFunc and wraps it into a http.Handler by means of calling serveAdmitFunc.
func admitFuncHandler(admit admitFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveAdmitFunc(w, r, admit)
	})
}
