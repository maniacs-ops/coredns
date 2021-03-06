package kubernetes

import (
	"sort"
	"testing"

	"github.com/coredns/coredns/middleware/pkg/dnsrecorder"
	"github.com/coredns/coredns/middleware/test"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
	"k8s.io/client-go/1.5/pkg/api"
)

type APIConnReverseTest struct{}

func (APIConnReverseTest) Run()                          { return }
func (APIConnReverseTest) Stop() error                   { return nil }
func (APIConnReverseTest) PodIndex(string) []interface{} { return nil }

func (APIConnReverseTest) ServiceList() []*api.Service {
	svcs := []*api.Service{
		{
			ObjectMeta: api.ObjectMeta{
				Name:      "svc1",
				Namespace: "testns",
			},
			Spec: api.ServiceSpec{
				ClusterIP: "192.168.1.100",
				Ports: []api.ServicePort{{
					Name:     "http",
					Protocol: "tcp",
					Port:     80,
				}},
			},
		},
	}
	return svcs
}

func (APIConnReverseTest) EndpointsList() api.EndpointsList {
	return api.EndpointsList{
		Items: []api.Endpoints{
			{
				Subsets: []api.EndpointSubset{
					{
						Addresses: []api.EndpointAddress{
							{
								IP:       "10.0.0.100",
								Hostname: "ep1a",
							},
						},
						Ports: []api.EndpointPort{
							{
								Port:     80,
								Protocol: "tcp",
								Name:     "http",
							},
						},
					},
				},
				ObjectMeta: api.ObjectMeta{
					Name:      "svc1",
					Namespace: "testns",
				},
			},
		},
	}
}

func (APIConnReverseTest) GetNodeByName(name string) (api.Node, error) {
	return api.Node{
		ObjectMeta: api.ObjectMeta{
			Name: "test.node.foo.bar",
		},
	}, nil
}

func TestReverse(t *testing.T) {

	k := Kubernetes{Zones: []string{"cluster.local.", "0.10.in-addr.arpa."}}
	k.interfaceAddrsFunc = localPodIP
	k.APIConn = &APIConnReverseTest{}

	tests := []test.Case{
		{
			Qname: "100.0.0.10.in-addr.arpa.", Qtype: dns.TypePTR,
			Rcode: dns.RcodeSuccess,
			Answer: []dns.RR{
				test.PTR("100.0.0.10.in-addr.arpa.      303    IN      PTR       ep1a.svc1.testns.svc.cluster.local."),
			},
		},
		{
			Qname: "101.0.0.10.in-addr.arpa.", Qtype: dns.TypePTR,
			Rcode: dns.RcodeSuccess,
			Ns: []dns.RR{
				test.SOA("0.10.in-addr.arpa.	300	IN	SOA	ns.dns.0.10.in-addr.arpa. hostmaster.0.10.in-addr.arpa. 1502782828 7200 1800 86400 60"),
			},
		},
	}

	ctx := context.TODO()
	for i, tc := range tests {
		r := tc.Msg()

		w := dnsrecorder.New(&test.ResponseWriter{})

		_, err := k.ServeDNS(ctx, w, r)
		if err != tc.Error {
			t.Errorf("Test %d: expected no error, got %v", i, err)
			return
		}
		if tc.Error != nil {
			continue
		}

		resp := w.Msg
		if resp == nil {
			t.Fatalf("Test %d: got nil message and no error for: %s %d", i, r.Question[0].Name, r.Question[0].Qtype)
		}

		sort.Sort(test.RRSet(resp.Answer))
		sort.Sort(test.RRSet(resp.Ns))
		sort.Sort(test.RRSet(resp.Extra))

		if !test.Header(t, tc, resp) {
			t.Logf("Test %d, received: %v", i, resp)
			continue
		}
		if !test.Section(t, tc, test.Answer, resp.Answer) {
			t.Logf("Test %d, received: %v", i, resp)
		}
		if !test.Section(t, tc, test.Ns, resp.Ns) {
			t.Logf("Test %d, received: %v", i, resp)
		}
		if !test.Section(t, tc, test.Extra, resp.Extra) {
			t.Logf("Test %d, received: %v", i, resp)
		}
	}
}
