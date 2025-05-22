# baremetal-ctl

[![CI](https://github.com/CarlosLaraFP/baremetal-ctl/actions/workflows/ci.yml/badge.svg)](https://github.com/CarlosLaraFP/baremetal-ctl/actions)

A Kubernetes-native provisioning platform with a gRPC infrastructure API for managing bare-metal compute nodes at scale.
Designed to simulate the infrastructure lifecycle challenges faced by modern GPU fleet operators like CoreWeave — built in Go using idiomatic controller-runtime patterns, gRPC, and CRDs.

---

## Overview
baremetal-ctl is a custom Kubernetes controller and gRPC API service that provisions and manages Linux-based compute nodes declaratively through custom resources. It’s designed to reflect how platform teams manage physical infrastructure behind the scenes while presenting a clean, self-service experience to users. This is the starting point in building a multi-tenant self-service GPU compute platform for AI workloads.

- Go + Kubernetes controller development
- gRPC infrastructure API design and implementation
- Declarative infra modeling through CRDs
- Status reconciliation and lifecycle automation
- Realistic simulation of bare-metal provisioning logic

Kubernetes API Server --> baremetal-ctl Controller (watches BareMetalNodeClaim) --> gRPC Infra Provisioner (handles Provision/Delete lifecycle) --> Simulated Linux Infra (logs, fake nodes, status updates)

---

## Key Features

- Go-based Kubernetes controller using controller-runtime
- Declarative CRD with full lifecycle management
- gRPC backend using google.golang.org/grpc and protobuf
- Controller <-> gRPC communication with reconcilable status
- Finalizers to trigger deprovisioning
- Status subresource updates and conditions
- Local simulation of real infrastructure workflows
- Prometheus metrics
- TLS-secured gRPC

---

## Core Concepts

### Custom Resource: BareMetalNodeClaim
Users submit BareMetalNodeClaim resources:
```
apiVersion: infra.example.org/v1alpha1
kind: BareMetalNodeClaim
metadata:
  name: node1
spec:
  cpu: "16"
  memory: "64Gi"
  disk: "1Ti"
  osImage: "ubuntu-22.04"
  zone: "rack-az1"
```
This represents a request for compute infrastructure. The controller handles reconciliation and backend orchestration.

### gRPC API
Provisioning is handled by an internal Go-based gRPC service:
```
rpc ProvisionNode(ProvisionRequest) returns (ProvisionResponse);
rpc DeleteNode(DeleteRequest) returns (DeleteResponse);
```
This layer simulates node provisioning and emits structured events and metrics.

---

## Why I Built This
I built this project to simulate the type of internal infrastructure platforms used by modern AI and compute-scale providers like CoreWeave. The goal was to demonstrate:

- Real-world controller development
- Understanding of infrastructure APIs (gRPC)
- Reconciliation-based automation
- Infrastructure abstraction and lifecycle clarity
- This project builds on my prior work with Crossplane and Kubernetes operators, and serves as a practical demonstration of end-to-end platform thinking.

---

## Next Steps

- Add REST microservices with client-side error-handling to simulate real distributed systems
- Integration with libvirt or kubevirt for local VM provisioning
- Expand CRD to support GPU scheduling, labels, taints
- Cost-effective multi-cloud vendor support (i.e. Karpenter-based EKS Auto with EC2 Spot node group)
- Add retries, TTLs, and lifecycle metrics
- GitOps-style event recording for observability

---

## Technologies Used

- Go 1.24+
- gRPC + Protocol Buffers
- Kubebuilder / controller-runtime
- Kubernetes CRDs & RBAC
- KinD for local testing
- Prometheus 

---

```
# Apply a node claim
kubectl apply -f examples/nodeclaim.yaml

# Watch controller logs
kubectl logs -f deployment/baremetal-controller

# View status
kubectl get baremetalnodeclaims.node.infra -o wide
```