package pomerium

import (
	"crypto/sha256"
	"encoding/json"
	"sort"
	"sync"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pomerium/ingress-controller/model"
)

type ingressConfigFingerprint [sha256.Size]byte

// ingressConfigCache tracks the exact Kubernetes inputs last applied for each
// Ingress. Status and other server-managed metadata are deliberately excluded.
type ingressConfigCache struct {
	mu          sync.RWMutex
	initialized bool
	applied     map[types.NamespacedName]ingressConfigFingerprint
}

func (c *ingressConfigCache) hit(name types.NamespacedName, fingerprint ingressConfigFingerprint) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	applied, ok := c.applied[name]
	return ok && applied == fingerprint
}

func (c *ingressConfigCache) contains(name types.NamespacedName) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.applied[name]
	return ok
}

func (c *ingressConfigCache) matches(fingerprints map[types.NamespacedName]ingressConfigFingerprint) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.initialized || len(c.applied) != len(fingerprints) {
		return false
	}
	for name, fingerprint := range fingerprints {
		if c.applied[name] != fingerprint {
			return false
		}
	}
	return true
}

func (c *ingressConfigCache) store(name types.NamespacedName, fingerprint ingressConfigFingerprint) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.applied == nil {
		c.applied = make(map[types.NamespacedName]ingressConfigFingerprint)
	}
	c.initialized = true
	c.applied[name] = fingerprint
}

func (c *ingressConfigCache) replace(fingerprints map[types.NamespacedName]ingressConfigFingerprint) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initialized = true
	c.applied = fingerprints
}

func (c *ingressConfigCache) delete(name types.NamespacedName) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.applied, name)
}

type ingressConfigFingerprintInput struct {
	AnnotationPrefix string                 `json:"annotationPrefix"`
	Ingress          ingressFingerprint     `json:"ingress"`
	Endpoints        []endpointsFingerprint `json:"endpoints"`
	Secrets          []secretFingerprint    `json:"secrets"`
	Services         []serviceFingerprint   `json:"services"`
}

type ingressFingerprint struct {
	UID         types.UID                `json:"uid"`
	Generation  int64                    `json:"generation"`
	Labels      map[string]string        `json:"labels"`
	Annotations map[string]string        `json:"annotations"`
	Spec        networkingv1.IngressSpec `json:"spec"`
}

type objectFingerprint struct {
	Namespace       string    `json:"namespace"`
	Name            string    `json:"name"`
	UID             types.UID `json:"uid"`
	ResourceVersion string    `json:"resourceVersion"`
}

type endpointsFingerprint struct {
	Object  objectFingerprint       `json:"object"`
	Subsets []corev1.EndpointSubset `json:"subsets"`
}

type secretFingerprint struct {
	Object objectFingerprint `json:"object"`
	Type   corev1.SecretType `json:"type"`
	Data   map[string][]byte `json:"data"`
}

type serviceFingerprint struct {
	Object objectFingerprint  `json:"object"`
	Spec   corev1.ServiceSpec `json:"spec"`
}

func fingerprintIngressConfigs(ics []*model.IngressConfig) (map[types.NamespacedName]ingressConfigFingerprint, error) {
	fingerprints := make(map[types.NamespacedName]ingressConfigFingerprint, len(ics))
	for _, ic := range ics {
		fingerprint, err := fingerprintIngressConfig(ic)
		if err != nil {
			return nil, err
		}
		fingerprints[ic.GetIngressNamespacedName()] = fingerprint
	}
	return fingerprints, nil
}

func fingerprintIngressConfig(ic *model.IngressConfig) (ingressConfigFingerprint, error) {
	input := ingressConfigFingerprintInput{
		AnnotationPrefix: ic.AnnotationPrefix,
		Ingress: ingressFingerprint{
			UID:         ic.Ingress.UID,
			Generation:  ic.Ingress.Generation,
			Labels:      ic.Ingress.Labels,
			Annotations: ic.Ingress.Annotations,
			Spec:        ic.Ingress.Spec,
		},
		Endpoints: make([]endpointsFingerprint, 0, len(ic.Endpoints)),
		Secrets:   make([]secretFingerprint, 0, len(ic.Secrets)),
		Services:  make([]serviceFingerprint, 0, len(ic.Services)),
	}

	for _, endpoint := range ic.Endpoints {
		input.Endpoints = append(input.Endpoints, endpointsFingerprint{
			Object: objectMetaFingerprint(endpoint.ObjectMeta), Subsets: endpoint.Subsets,
		})
	}
	for _, secret := range ic.Secrets {
		input.Secrets = append(input.Secrets, secretFingerprint{
			Object: objectMetaFingerprint(secret.ObjectMeta), Type: secret.Type, Data: secret.Data,
		})
	}
	for _, service := range ic.Services {
		input.Services = append(input.Services, serviceFingerprint{
			Object: objectMetaFingerprint(service.ObjectMeta), Spec: service.Spec,
		})
	}

	sort.Slice(input.Endpoints, func(i, j int) bool { return objectLess(input.Endpoints[i].Object, input.Endpoints[j].Object) })
	sort.Slice(input.Secrets, func(i, j int) bool { return objectLess(input.Secrets[i].Object, input.Secrets[j].Object) })
	sort.Slice(input.Services, func(i, j int) bool { return objectLess(input.Services[i].Object, input.Services[j].Object) })

	b, err := json.Marshal(input)
	if err != nil {
		return ingressConfigFingerprint{}, err
	}
	return sha256.Sum256(b), nil
}

func objectMetaFingerprint(meta metav1.ObjectMeta) objectFingerprint {
	return objectFingerprint{
		Namespace: meta.Namespace, Name: meta.Name, UID: meta.UID, ResourceVersion: meta.ResourceVersion,
	}
}

func objectLess(a, b objectFingerprint) bool {
	if a.Namespace != b.Namespace {
		return a.Namespace < b.Namespace
	}
	return a.Name < b.Name
}
