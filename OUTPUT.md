# webhook-server

## Usage
Validate the webhook-server, our admission-controller service is up.
```shell
$ kc get pods
NAME                                   READY   STATUS    RESTARTS   AGE
vault-agent-injector-957c98b8d-7m2nk   1/1     Running   2          14h
webhook-server-6cc5bdf5d8-h7nnk        2/2     Running   0          8s
```

Now it is time to validate our service and if it is performing enforcements correctly based on pod annotations. Use the deployments in the `samples` folder to validate the service.
On trying to create a deployment with missing annotations --
```shell
ajayaraman at C02DJ1LXML7J in ~/go/src/admission-controller-webhook-demo
$ kc create -f samples/hello-app-missing-annotations.yaml
Error from server: error when creating "samples/hello-app-missing-annotations.yaml": admission webhook "webhook-server.default.svc" denied the request: the submitted Pods are missing required annotations: map[app.kubernetes.io/component:key was not found app.kubernetes.io/name:key was not found]
```

On trying to create a deployment with proper annotations --
```shell
ajayaraman at C02DJ1LXML7J in ~/go/src/admission-controller-webhook-demo
$ kc create -f samples/hello-app-with-annotations.yaml
deployment.apps/hello-app created

$ kc get pods
NAME                                   READY   STATUS    RESTARTS   AGE
hello-app-9647d7fff-cppdd              1/1     Running   0          15s
vault-agent-injector-957c98b8d-7m2nk   1/1     Running   2          14h
webhook-server-6cc5bdf5d8-h7nnk        2/2     Running   0          3m36s
```

```shell
$ kc delete -f deployment/deployment.yaml
deployment.apps "webhook-server" deleted
service "webhook-server" deleted
Warning: admissionregistration.k8s.io/v1beta1 ValidatingWebhookConfiguration is deprecated in v1.16+, unavailable in v1.22+; use admissionregistration.k8s.io/v1 ValidatingWebhookConfiguration
validatingwebhookconfiguration.admissionregistration.k8s.io "webhook-server" deleted

```

## Troubleshooting and Tips
### Vault service
* Verify your secret is stored in vault at correct location
```shell
$ vault read -format json secret/data/webhook-server/config | jq ".data.data"
{
  "cert": "<base64-encoded-cert>",
  "key": "<base64-encoded-key>"
}
```

### admission-controller-webhook-demo
```shell
ajayaraman at C02DJ1LXML7J in ~/go/src/admission-controller-webhook-demo
$ go test ./... -v
=== RUN   TestEnforcePodAnnotations
=== RUN   TestEnforcePodAnnotations/Allow_Pod_with_required_annotations
2021/01/27 09:29:49 Enforcing annotations for Pod in ns
=== RUN   TestEnforcePodAnnotations/Deny_Pod_with_required_annotations
2021/01/27 09:29:49 Enforcing annotations for Pod in ns
--- PASS: TestEnforcePodAnnotations (0.01s)
    --- PASS: TestEnforcePodAnnotations/Allow_Pod_with_required_annotations (0.01s)
    --- PASS: TestEnforcePodAnnotations/Deny_Pod_with_required_annotations (0.00s)
PASS
ok  	admission-controller-webhook-demo/cmd/webhook-server	0.214s

```

### webhook-server
#### Logs
You should see the following logs when the controller service is working as expected
```shell
$ kubectl logs -f -l app=webhook-server -c webhook-server
2021/01/27 17:25:16 Webhook request handled successfully
2021/01/27 17:26:13 Handling webhook request ...
2021/01/27 17:26:13 Enforcing annotations for Deployment in ns default
2021/01/27 17:26:13 Webhook request handled successfully
2021/01/27 17:26:13 Handling webhook request ...
2021/01/27 17:26:13 Enforcing annotations for ReplicaSet in ns default
2021/01/27 17:26:13 Webhook request handled successfully
2021/01/27 17:26:13 Handling webhook request ...
2021/01/27 17:26:13 Enforcing annotations for Pod in ns default
2021/01/27 17:26:14 Webhook request handled successfully
```

### Compatible software versions

```shell
ajayaraman at C02DJ1LXML7J in ~/go/src/cfssl/bin on master
$ ./cfssl version
Version: 1.5.0
Runtime: go1.15.7

$ minikube version
minikube version: v1.16.0
commit: 9f1e482427589ff8451c4723b6ba53bb9742fbb1

$ go version
go version go1.15.7 darwin/amd64

$ vault version
Vault v1.6.1 (6d2db3f033e02e70202bef9ec896360062b88b03)

$ helm version
version.BuildInfo{Version:"v3.5.0", GitCommit:"32c22239423b3b4ba6706d450bd044baffdcf9e6", GitTreeState:"dirty", GoVersion:"go1.15.6"}

$ docker version
Client: Docker Engine - Community
 Cloud integration: 1.0.7
 Version:           20.10.2
 API version:       1.41
 Go version:        go1.13.15
 Git commit:        2291f61
 Built:             Mon Dec 28 16:12:42 2020
 OS/Arch:           darwin/amd64
 Context:           default
 Experimental:      true
```
