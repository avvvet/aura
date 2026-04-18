package model

import "time"

// ClusterSnapshot is the complete picture of a cluster at a point in time
type ClusterSnapshot struct {
	CapturedAt  time.Time
	ClusterName string
	Context     string

	Nodes       []Node
	Namespaces  []Namespace
	Pods        []Pod
	Deployments []Deployment
	Services    []Service
	Ingresses   []Ingress
	PVCs        []PVC

	Analysis        []string
	SecuritySignals SecuritySignals
	CostSignals     CostSignals
	Errors          []string
}

// SecuritySignals represents security related findings
type SecuritySignals struct {
	PrivilegedContainers    []string
	ContainersRunningAsRoot []string
	SecretsInEnvVars        []string
	HostNetworkPods         []string
	NoSecurityContext       []string
	LatestImageTags         []string
	IngressesWithoutTLS     []string
	NamespacesWithoutNetPol []string
}

// Node represents a cluster node and its status
type Node struct {
	Name           string
	Status         string
	Roles          string
	Age            string
	Version        string
	CPUCapacity    string
	MemoryCapacity string
	CPUUsage       string
	MemoryUsage    string
}

// Namespace represents a cluster namespace
type Namespace struct {
	Name   string
	Status string
	Age    string
}

// Pod represents a pod and its status
type Pod struct {
	Name          string
	Namespace     string
	Status        string
	Ready         string
	Restarts      int32
	Age           string
	Node          string
	OwnerKind     string
	OwnerName     string
	CPURequest    string
	MemoryRequest string
	CPULimit      string
	MemoryLimit   string
}

// Deployment represents a deployment and its health
type Deployment struct {
	Name      string
	Namespace string
	Ready     string
	UpToDate  int32
	Available int32
	Age       string
}

// Service represents a service and its exposure
type Service struct {
	Name       string
	Namespace  string
	Type       string
	ClusterIP  string
	ExternalIP string
	Ports      string
	Age        string
}

// Ingress represents an ingress and its rules
type Ingress struct {
	Name      string
	Namespace string
	Class     string
	Hosts     string
	Address   string
	Ports     string
	Age       string
}

// PVC represents a persistent volume claim
type PVC struct {
	Name         string
	Namespace    string
	Status       string
	Volume       string
	Capacity     string
	AccessModes  string
	StorageClass string
	Age          string
}

// CostSignals represents cost related signals derived from cluster state
type CostSignals struct {
	PodsWithNoLimits     []string
	UnattachedPVCs       []string
	IdleNamespaces       []string
	OverprovisionedNodes []string
}

// NewSnapshot creates a new empty ClusterSnapshot
func NewSnapshot(clusterName, context string) *ClusterSnapshot {
	return &ClusterSnapshot{
		CapturedAt:  time.Now(),
		ClusterName: clusterName,
		Context:     context,
	}
}
