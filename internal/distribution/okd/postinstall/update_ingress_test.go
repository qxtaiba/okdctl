package postinstall

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildLBIngressController_PreservesFields(t *testing.T) {
	cases := []struct {
		name        string
		icJSON      json.RawMessage
		hasReplicas bool
		hasCert     bool
		hasRoute    bool
		hasAdmit    bool
		hasNode     bool
	}{
		{
			name: "all optional fields present",
			icJSON: json.RawMessage(`{
				"metadata":{"name":"custom","namespace":"openshift-ingress-operator"},
				"spec":{
					"domain":"apps.example.com",
					"replicas":3,
					"defaultCertificate":{"name":"my-cert"},
					"routeSelector":{"matchLabels":{"env":"prod"}},
					"routeAdmission":{"wildcardPolicy":"WildcardsDisallowed"},
					"nodePlacement":{"nodeSelector":{"matchLabels":{"node-role.kubernetes.io/worker":""}}}
				}
			}`),
			hasReplicas: true,
			hasCert:     true,
			hasRoute:    true,
			hasAdmit:    true,
			hasNode:     true,
		},
		{
			name: "absent optional fields stay omitted",
			icJSON: json.RawMessage(`{
				"metadata":{"name":"minimal","namespace":"openshift-ingress-operator"},
				"spec":{"domain":"apps.minimal.test"}
			}`),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ic := &ingressControllerInfo{RawJSON: tc.icJSON}
			out, err := buildLBIngressController(ic)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var parsed struct {
				Spec struct {
					EndpointPublishingStrategy struct {
						Type string `json:"type"`
					} `json:"endpointPublishingStrategy"`
					Domain             string           `json:"domain"`
					Replicas           *int32           `json:"replicas"`
					DefaultCertificate *json.RawMessage `json:"defaultCertificate"`
					RouteSelector      *json.RawMessage `json:"routeSelector"`
					RouteAdmission     *json.RawMessage `json:"routeAdmission"`
					NodePlacement      *json.RawMessage `json:"nodePlacement"`
				} `json:"spec"`
				Metadata struct {
					Namespace string `json:"namespace"`
				} `json:"metadata"`
			}
			if err := json.Unmarshal([]byte(out), &parsed); err != nil {
				t.Fatalf("output is not valid JSON: %v", err)
			}

			if parsed.Spec.EndpointPublishingStrategy.Type != "LoadBalancerService" {
				t.Errorf("Type = %q; want LoadBalancerService", parsed.Spec.EndpointPublishingStrategy.Type)
			}
			if parsed.Metadata.Namespace == "" {
				t.Error("Namespace must not be empty")
			}

			checkOpt := func(name string, want bool, got bool) {
				if want && !got {
					t.Errorf("%s expected but absent", name)
				}
				if !want && got {
					t.Errorf("%s unexpected but present", name)
				}
			}
			checkOpt("Replicas", tc.hasReplicas, parsed.Spec.Replicas != nil)
			checkOpt("DefaultCertificate", tc.hasCert, parsed.Spec.DefaultCertificate != nil)
			checkOpt("RouteSelector", tc.hasRoute, parsed.Spec.RouteSelector != nil)
			checkOpt("RouteAdmission", tc.hasAdmit, parsed.Spec.RouteAdmission != nil)
			checkOpt("NodePlacement", tc.hasNode, parsed.Spec.NodePlacement != nil)
		})
	}
}

func TestBuildRollbackJSON_StripsServerFields(t *testing.T) {
	icJSON := json.RawMessage(`{
		"apiVersion":"operator.openshift.io/v1",
		"kind":"IngressController",
		"metadata":{
			"name":"default",
			"namespace":"openshift-ingress-operator",
			"creationTimestamp":"2024-01-01T00:00:00Z",
			"generation":3,
			"resourceVersion":"99999",
			"selfLink":"/apis/operator.openshift.io/v1/...",
			"uid":"aaaa-bbbb-cccc",
			"managedFields":[{"manager":"oc","operation":"Apply"}]
		},
		"spec":{"domain":"apps.cluster.local"},
		"status":{"availableReplicas":2}
	}`)

	ic := &ingressControllerInfo{RawJSON: icJSON}
	out, err := buildRollbackJSON(ic)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, field := range []string{"creationTimestamp", "generation", "resourceVersion", "selfLink", "managedFields"} {
		if strings.Contains(out, field) {
			t.Errorf("rollback output still contains %q:\n%s", field, out)
		}
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatalf("rollback output is not valid JSON: %v", err)
	}
	if _, ok := obj["status"]; ok {
		t.Error("status key must be stripped but is present")
	}
}

func TestBuildLBIngressController_TypeIsLoadBalancerService(t *testing.T) {
	ic := &ingressControllerInfo{
		RawJSON: json.RawMessage(`{
			"metadata":{"name":"default","namespace":"openshift-ingress-operator"},
			"spec":{
				"domain":"apps.cluster.local",
				"endpointPublishingStrategy":{"type":"HostNetwork"}
			}
		}`),
	}
	out, err := buildLBIngressController(ic)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed struct {
		Spec struct {
			EndpointPublishingStrategy struct {
				Type string `json:"type"`
			} `json:"endpointPublishingStrategy"`
		} `json:"spec"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed.Spec.EndpointPublishingStrategy.Type != "LoadBalancerService" {
		t.Errorf("Type = %q; want LoadBalancerService", parsed.Spec.EndpointPublishingStrategy.Type)
	}
}
