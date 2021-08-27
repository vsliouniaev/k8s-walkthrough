# Set-Up
- Install docker. For windows, this is docker desktop (don't know if they fixed the choco package, so just use the website)
- [Install go](https://golang.org/dl/)
- [Install "Kubernetes In Docker"](https://sigs.k8s.io/kind)
- Create a cluster `kind create cluster`
- If something gets wrecked, simplest thing is to just delete it and start again: `kind delete cluster`

# Basic commands
_This guide uses long-form commands the first time and then gradually tries to cut down on the syntax. If you work with kubernetes a lot there are various short-cuts you can create for yourself or find terminal extensions. You can get some from `terminal-cfg`_

We use `kubectl` to interact with the cluster. This program has a YAML file in `~/.kube/config`, which stores connection information.

The basic building-block you use to get things done is called a `pod`. This is one or more containers running together on a `node` (vm/physical machine.)

You can poke around to look at what's running in the control-plane of the cluster:
```sh
kubectl get pods --namespace kube-system
```
Depending on what is running Kubernetes (AKS / GKE / KIND), you will probably see different stuff in here.

You can inspect one of the pod configurations with
```sh
kubectl get pod kube-controller-manager-kind-control-plane --output yaml -nkube-system
```

Everything here except for labels uniquely identifies a resource. Labels let resources reference one-another:
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: kube-controller-manager-kind-control-plane
  namespace: kube-system
  labels: {}
```

This is where the configuration of your containers goes:
```yaml
spec:
  containers: []
```

# Running a Pod
You can make your own docker image or use the code here to make one.
```sh
GOOS=linux GOARCH=amd64 go build demo.go
docker build . -t demo:1
```
If your image is in a docker repository you don't have to do this next step:
```sh
kind load docker-image demo:1
```

Let's create a new namespace so we're not in the default one
```
kubectl create namespace test
kubectl config set-context --current --namespace=test
```

We need to write the pod definition next:
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: pod-1
spec:
  containers:
    - name: demo
      image: demo:1
```

The fastest way to work with these is to save them to a file, and then use `kubectl apply` command.

```sh
kubectl apply -f https://raw.githubusercontent.com/vsliouniaev/k8s-walkthrough/master/pod-1.yaml
```

You can now check the basic status by running `kubectl get po`. Or get more information about the events associated with the pod using `kubectl describe po`. You can look at the log output with `kubectl logs pod-1 --follow`

This is the most basic pod configuration, so it doesn't do much. Let's add another container, expose some ports and give it some command-line options:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: pod-2
spec:
  containers:
  - name: one
    image: demo:1
    args:
    - --pod=2
    - --container=container1
    - --listen=:8080
    - --call=http://localhost:8081
    ports:
    - containerPort: 8080
  - name: two
    image: demo:1
    args:
      - --pod=2
      - --container=container2
      - --listen=:8081
      - --call=http://localhost:8080
```

```sh
kubectl apply -f https://raw.githubusercontent.com/vsliouniaev/k8s-walkthrough/master/pod-2.yaml
kubectl logs pod-2
```
When you run it you can see it will print its id as pod:1 conainer:container1, the arguments we specified.

You can bind a pod port directly to localhost and poke around:
```sh
kubectl port-forward pod-2 8080
```

Going to localhost:8080 will give out the pod's id. Going to http://localhost:8080/call will make this container call the other container on the pod's internal network - it makes an http request to "http://localhost:8080 and returns the result. You can build mini-environments like this that talk internally, take care of routing, retries, TLS termination, authentication/authorization. It lets you replace the dependency-as-code pattern into the "sidecar" pattern.

# Pod Filesystem

Similar to how you have an isolated network in the pod that containers can use, you can also interact with files and share them between containers.


```yaml
apiVersion: v1
kind: Pod
metadata:
  name: pod-3
spec:
  containers:
  - name: one
    image: demo:1
    args:
    - --pod=3
    - --container=container1
    - --listen=:8080
    - --call=http://localhost:8081
    ports:
    - containerPort: 8080
    volumeMounts:
      - name: scratch
        mountPath: /tmp
      - name: config
        mountPath: /etc/app-config
  - name: two
    image: demo:1
    args:
      - --pod=3
      - --container=container2
      - --listen=:8081
      - --call=http://localhost:8080
    ports:
    - containerPort: 8081
    volumeMounts:
      - name: scratch
        mountPath: /tmp
      - name: config
        mountPath: /etc/app-config
  volumes:
  - name: scratch
    emptyDir: {}
  - name: config
    configMap:
      defaultMode: 420
      name: app-config
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
data:
  file.conf: "Some file content!"
```

```sh
kubectl apply -f https://raw.githubusercontent.com/vsliouniaev/k8s-walkthrough/master/pod-3.yaml
```

You can port-forward to either container and
- Get the contents of the config file on http://localhost:8080/cfg
- Write some files into the shared filesystem with http://localhost:8080/fs?<filename>=<content>&<filename2>=<content2>
- Read some files using http://localhost:8080/fs?<filename>&<filename2>

# Deployments
First, a bit about Kubernetes internals. Kubernetes configuration defines the desired state of the cluster. A lot of these configuration `kind`s (nothing to do with the way we're running kubernetes in docker) come out of the box, like pods, deployments, secrets, configmaps and a huge list of others. Kubernetes then tries to get this configuration to run using its internals:
1. Linearizable database. This is `etcd`, but could theoretically be any db that satisfies this criterion. For example Cosmos in its strictest consistency mode. This stores the cluster state. Only the API server talks to this.
2. API server. This talks to the db, enforces the Kubernetes API. It can be extended using "admission webhooks" which can mutate and perform custom validation before you get the resources into etcd and make them available for everything else.
3. Kubelets. These run on each node, manipulate docker, iptables etc. They talk to the API server and reconcile the state of the node in accordance to what they get from the API server.
4. Controllers. These things talk to the api server and manipulate resources in response to events in the cluster or the outside world.

A pod by itself is of limited use. It always exists on a node and never moves. You can't update the configuration without some down-time involved in your running application. We can get multiple pods to run by using a `deployment`
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: deployment-1
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: demo-1
  strategy:
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 1
    type: RollingUpdate
  template:
    metadata:
      labels:
        app.kubernetes.io/name: demo-1
    spec:
      containers:
      - name: one
        image: demo:1
        args:
        - --pod=$(POD_NAME)
        - --container=container1
        - --listen=:8080
        - --call=http://localhost:8081
        ports:
        - containerPort: 8080
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: metadata.name
      - name: two
        image: demo:1
        args:
          - --pod=$(POD_NAME)
          - --container=container2
          - --listen=:8081
          - --call=http://localhost:8080
        ports:
        - containerPort: 8081
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: metadata.name
```

```sh
kubectl apply -f https://raw.githubusercontent.com/vsliouniaev/k8s-walkthrough/master/deployment-1.yaml
```

If you `kubectl edit` the deployment to increase the number of `replicas`, more pods will be created. If you change the args or other properties of the `spec`, new pods will be created before old ones are cleaned up.

# Services

It's all good running a bunch of replicas, but how do we communicate with them in a way that doesn't require us to call each one directly? Kubernetes uses low-level network routing to take care of this, doing it all with what's commonly called "iptables magic". Other solutions (like DNS for example), just wouldn't be fast enough at updating routing informaiton.

Let's add a service that targets the deployment. Note the `selector` here matches the labels we put on the pod spec in the `deployment`

```yaml
apiVersion: v1
kind: Service
metadata:
  name: demo-service
spec:
  ports:
  - port: 80
    targetPort: 8080
  selector:
    app.kubernetes.io/name: demo-1
```

```sh
kubectl apply -f https://raw.githubusercontent.com/vsliouniaev/k8s-walkthrough/master/service-1.yaml
```

We can port-forward to the service, but this is actually an alias for "find me a matching pod and port-forward to that", so it won't quite give us the load-balancing behaviour we want. Let's instead check what happens inside the cluster when we make remote calls:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: pod-4
spec:
  containers:
  - name: remote-caller
    image: demo:1
    args:
    - --call=http://demo-service
    ports:
    - containerPort: 8080
```

```sh
kubectl apply -f https://raw.githubusercontent.com/vsliouniaev/k8s-walkthrough/master/pod-4.yaml

kubectl port-forward pod-4 8080
```

This time if we access http://localhost:8080/call we'll hit one of the service pods. Kind seems to create sticky LB behaviour when I was testing it, but your request could go to any one of the pods that are selected by the `app.kubernetes.io/name: demo-1` selector on the service. You can `kubectl delete` the pod you keep hitting and the next request will go to some other pod, while a replacement pod is created.

# Exposing Services to the World

There are lots of ways to actually get your containers to talk to the outside world, complete with authentication, authorization, TLS certificate provisioning etc. These depend on the features given to you out-of-the-box by the cluster provider or addons you install. Let's do the quick an dirty setup here by using [nginx ingress](https://kind.sigs.k8s.io/docs/user/ingress)

We'll need to expose some ports from the containers to get this to work, so let's re-create the cluster with those exposed and run nginx on it:

```sh
kind delete cluster
curl https://raw.githubusercontent.com/vsliouniaev/k8s-walkthrough/master/kind.yaml -o kind.yaml
kind create cluster --config kind.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/master/deploy/static/provider/kind/deploy.yaml
```

Now we have a bunch of resources in the `ingress-nginx` namespace:
```
kubectl get all -ningress-nginx
```

One of the the `kind`s you will find there is a job, which can be used to create pods periodically on a schedule. In the case of this set-up, they were used to generate some certificates and do some in-cluster automation to get nginx working correctly.

You will also see a `replicaset`, which is a resource used by `deplyment` behind the scenes to perform changes to sets of pods.

Let's re-create the deployment, service and add an `ingress` resource

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-demo-app
spec:
  replicas: 3
  selector:
    matchLabels:
      app.kubernetes.io/name: my-demo-app
  strategy:
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 1
    type: RollingUpdate
  template:
    metadata:
      labels:
        app.kubernetes.io/name: my-demo-app
    spec:
      containers:
      - name: demo
        image: demo:1
        args:
        - --pod=$(POD_NAME)
        - --container=demo
        - --call=http://my-demo-app
        ports:
        - containerPort: 8080
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: metadata.name
---
apiVersion: v1
kind: Service
metadata:
  name: my-demo-app
  labels:
    app.kubernetes.io/name: my-demo-app
spec:
  ports:
  - port: 80
    targetPort: 8080
  selector:
    app.kubernetes.io/name: my-demo-app
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    kubernetes.io/ingress.class: nginx
  name: my-demo-app
spec:
  rules:
  - http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: my-demo-app
            port:
              number: 80

```

```sh
kubectl apply -f https://raw.githubusercontent.com/vsliouniaev/k8s-walkthrough/master/deployment-with-service-and-ingress.yaml
```

Now you can just go to http://localhost and see your stuff running.

# Helm and "The Rest of the Owl"

There are several tools that will let you template out Kubernetes configuration and customize it without having to hand-craft the configs yourself each time. One of these is [Helm](https://helm.sh). The latest version Helm 3 is quite new and stopped prescribing a default repository of configs, so we'll have to add it. All OSs should have [packages for helm](https://helm.sh/docs/intro/install/), but I tend to just download a github release, since it's just one binary.

Add the old helm chart repository to helm 3:

```sh
helm repo add stable https://kubernetes-charts.storage.googleapis.com/
```

Let's install a project I'm familiar with and poke around. This installs the most common cluster monitoring component, which uses [coreos/prometheus-operator](https://github.com/coreos/prometheus-operator) to run [prometheus](https://prometheus.io/) as well as a load of other components to gather metrics from the cluster, run some rules on them and trigger alerts when things go bad:

```sh
kubectl create namespace monitoring
kubectl config set-context --current --namespace=monitoring
helm3 upgrade prom-op stable/prometheus-operator --install --namespace monitoring
```

There's a huge amount of stuff going on here, but you can port-forward to the "prometheus" and "alertmanager" pods to get to the interesting stuff.
