# webhook-server

###### A basic kubernetes admission controller that enforces pod creation based on proper labels and annotations

## Setup and Installation

A cluster on which this example can be tested must be running Kubernetes 1.9.0 or above, with the admissionregistration.k8s.io/v1beta1 API enabled.
You can verify that by observing that the following command produces a non-empty output:

[NOTE: `kc` is simply a local alias for `kubectl`]
```
kc version
Client Version: version.Info{Major:"1", Minor:"20", GitVersion:"v1.20.2", GitCommit:"faecb196815e248d3ecfb03c680a4507229c2a56", GitTreeState:"clean", BuildDate:"2021-01-14T05:14:17Z", GoVersion:"go1.15.6", Compiler:"gc", Platform:"darwin/amd64"}
Server Version: version.Info{Major:"1", Minor:"20", GitVersion:"v1.20.0", GitCommit:"af46c47ce925f4c4ad5cc8d1fca46c7b77d13b38", GitTreeState:"clean", BuildDate:"2020-12-08T17:51:19Z", GoVersion:"go1.15.5", Compiler:"gc", Platform:"linux/amd64"}

kc api-versions | grep admissionregistration.k8s.io/v1beta1
admissionregistration.k8s.io/v1beta1

kc -n kube-system describe pod kube-apiserver-minikube | grep enable-admission-plugins [NOTE: Pod name "kube-apiserver-<X> can change based on the type of k8s environment]
--enable-admission-plugins=NamespaceLifecycle,LimitRanger,ServiceAccount,DefaultStorageClass,DefaultTolerationSeconds,NodeRestriction,MutatingAdmissionWebhook,ValidatingAdmissionWebhook,ResourceQuota
```

Installation of this admission controller consists of these main steps:
1. Generate a TLS keypair for the admission controller by issuing a `CertificateSigningRequest` against the Kubernetes cluster, and obtain the CA certificate from the k8s cluster.
2. Using Vault to store the secrets, in our case the private key and the cert used by the admission controller service to interact with the kube api server
3. Building the admission controller image by running `make` and have a docker image in your local docker repository ready for deployment.
4. Creating a `Deployment` and a `Service` that makes the admission controller available to the cluster.

### Generating TLS certs

Next, we are trying to serve the webhook service via TLS. Kubernetes only allows HTTPS (TLS) communication to Admission Controllers, whether in-cluster or hosted externally—and make the key & certificate available as a Secret within your cluster.
(For demo purposes, as self-signed certificates require you to provide a `.webhooks.clientConfig.caBundle` for verification.)

Follow below steps to generate a TLS keypair for the admission controller by issuing a CertificateSigningRequest against the Kubernetes cluster, and obtain the CA certificate from the k8s cluster.
[For more information on managing TLS certs on Kubernetes - https://kubernetes.io/docs/tasks/tls/managing-tls-in-a-cluster/#create-a-certificate-signing-request]

* Install `cfssl` (https://kubernetes.io/docs/tasks/tls/managing-tls-in-a-cluster/#download-and-install-cfssl)
* Generate a private key and certificate signing request (or CSR) using the `certs/generate-csr.yaml` file
```yaml
cat <<EOF | ./cfssl genkey - | ./cfssljson -bare server
{
  "hosts": [
    "webhook-server.default.svc",
    "webhook-server.default.svc.cluster.local"
  ],
  "CN": "system:node:webhook-server.default.svc",
  "key": {
    "algo": "ecdsa",
    "size": 256
  },
  "names": [
   {
     "O": "system:nodes"
   }
  ]
}
EOF
```

* Create a Certificate Signing Request object to send to the Kubernetes API using `certs/csr.yaml`
```yaml
cat <<EOF | kubectl apply -f -
apiVersion: certificates.k8s.io/v1
kind: CertificateSigningRequest
metadata:
  name: webhook-server.default
spec:
  request: $(cat server.csr | base64 | tr -d '\n')
  signerName: kubernetes.io/kubelet-serving
  usages:
  - digital signature
  - key encipherment
  - server auth
EOF
```

* Get your CSR approved
```shell
kc certificate approve webhook-server.default
certificatesigningrequest.certificates.k8s.io/webhook-server.default approved
```

* CSR should now be Approved and Issued
```shell
kc get csr
NAME                          AGE   SIGNERNAME                      REQUESTOR       CONDITION
webhook-server.default   17m   kubernetes.io/kubelet-serving   minikube-user   Approved,Issued
```
```yaml
kubectl get csr webhook-server.default --output=jsonpath="{.status}" | jq .
{
  "certificate": "LS0tLS1C<truncated>",
  "conditions": [
    {
      "lastTransitionTime": "2021-01-24T19:27:08Z",
      "lastUpdateTime": "2021-01-24T19:27:08Z",
      "message": "This CSR was approved by kubectl certificate approve.",
      "reason": "KubectlApprove",
      "status": "True",
      "type": "Approved"
    }
  ]
}
```

* Retrieve the CSR to use as secret
```shell
kc get csr webhook-server.default -o jsonpath='{.status.certificate}' | base64 --decode > server.crt
```

* Now we can use the `certs/server.crt` and `certs/server-key.pem` for our admission controller to work in our kubernetes namespace.

### Setting up Vault and adding certs to Vault Secret Store

You can follow steps from here to install and start Vault - https://learn.hashicorp.com/tutorials/vault/kubernetes-external-vault

#### Start Vault Service

* Vault can be accessed at a`http://0.0.0.0:8200` to address pods within minikube k8s cluster we will use `EXTERNAL_VAULT_ADDR`
  `EXTERNAL_VAULT_ADDR=$(minikube ssh "dig +short host.docker.internal")`

* Login to vault `vault login root`

* We will store our private key and certs `base64 -in certs/server.crt` and `base64 -in certs/server-key.pem` on Vault in base64 encoded format
```shell
vault kv put secret/webhook-server/config key='<base64-encoded-key>' cert='<base64-encoded-cert>'
```

* Create a Service Account that will bind our admission controller service with vault service
```yaml
$ cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: internal-app
EOF
```

* Create an external vault service
  [https://learn.hashicorp.com/tutorials/vault/kubernetes-external-vault#deploy-service-and-endpoints-to-address-an-external-vault]
```yaml
$ cat <<EOF | kubectl apply -f -
---
apiVersion: v1
kind: Service
metadata:
  name: external-vault
  namespace: default
spec:
  ports:
  - protocol: TCP
    port: 8200
---
apiVersion: v1
kind: Endpoints
metadata:
  name: external-vault
subsets:
  - addresses:
      - ip: $EXTERNAL_VAULT_ADDR
    ports:
      - port: 8200
EOF
```

* We will use vault helm charts to install a vault agent that will retrieve secrets for our admission controller service.

* Create a service account, secret, and ClusterRoleBinding with the necessary permissions to allow Vault to perform token reviews with Kubernetes.
[https://learn.hashicorp.com/tutorials/vault/kubernetes-external-vault#define-a-kubernetes-service-account]

* Configure Kubernetes authentication
  [https://learn.hashicorp.com/tutorials/vault/kubernetes-external-vault#configure-kubernetes-authentication]
  [NOTE: While configuring authentication create policies and roles specific to the webhook-server]
```shell
$ vault policy write webhook-server - <<EOF
path "secret/data/webhook-server/config" {
capabilities = ["read"]
}
EOF
```

```shell
$ vault write auth/kubernetes/role/webhook-server-app \
        bound_service_account_names=internal-app \
        bound_service_account_namespaces=default \
        policies=webhook-server \
        ttl=24h
```

* Finally, install the vault helm chart
[https://learn.hashicorp.com/tutorials/vault/kubernetes-external-vault#install-the-vault-helm-chart]

You should end up with the vault injector pod and services running in your `default` namespace
```shell
$ kc get pods
NAME                                   READY   STATUS    RESTARTS   AGE
vault-agent-injector-957c98b8d-7m2nk   1/1     Running   2          14h
```

```shell
$ kc get svc
NAME                       TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)    AGE
external-vault             ClusterIP   10.97.109.50     <none>        8200/TCP   14h
kubernetes                 ClusterIP   10.96.0.1        <none>        443/TCP    5d22h
vault-agent-injector-svc   ClusterIP   10.102.195.150   <none>        443/TCP    14h
```

### Building admission controller binary

[NOTE: We will use your docker local repository to host the binaries]
* Setup minikube docker-env access by running `eval $(minikube -p minikube docker-env)`

```shell
ajayaraman at C02DJ1LXML7J in ~/go/src/admission-controller-webhook-demo
$ make
CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o image/webhook-server ./cmd/webhook-server
docker build -t ajayaraman/basic-admission-controller:latest image/
Sending build context to Docker daemon  15.96MB
Step 1/4 : FROM busybox
 ---> b97242f89c8a
Step 2/4 : EXPOSE 8443
 ---> Using cache
 ---> 0cd69ea1875f
Step 3/4 : COPY ./webhook-server /
 ---> 08923ee26706
Step 4/4 : ENTRYPOINT ["/webhook-server"]
 ---> Running in 38350afd0a7e
Removing intermediate container 38350afd0a7e
 ---> a5f678bab65a
Successfully built a5f678bab65a
Successfully tagged ajayaraman/basic-admission-controller:latest
```

### Deploy the basic admission controller service
Now its finally time to deploy our service
```shell
ajayaraman at C02DJ1LXML7J in ~/go/src/admission-controller-webhook-demo
$ kc create -f deployment/deployment.yaml
```

## Usage
The OUTPUT.MD files has more details on how to use the service and other helpful tips.
