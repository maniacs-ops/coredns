package kubernetes

import (
	"github.com/coredns/coredns/middleware/pkg/dnsutil"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

type recordRequest struct {
	// The named port from the kubernetes DNS spec, this is the service part (think _https) from a well formed
	// SRV record.
	port string
	// The protocol is usually _udp or _tcp (if set), and comes from the protocol part of a well formed
	// SRV record.
	protocol  string
	endpoint  string
	service   string
	namespace string
	// A each name can be for a pod or a service, here we track what we've seen. This value is true for
	// pods and false for services. If we ever need to extend this well use a typed value.
	podOrSvc string
	zone     string
}

// parseRequest parses the qname to find all the elements we need for querying k8s.
func (k *Kubernetes) parseRequest(state request.Request) (r recordRequest, err error) {
	// 3 Possible cases: TODO(chris): remove federations comments here.
	//   SRV Request: _port._protocol.service.namespace.[federation.]type.zone
	//   A Request (endpoint): endpoint.service.namespace.[federation.]type.zone
	//   A Request (service): service.namespace.[federation.]type.zone

	base, _ := dnsutil.TrimZone(state.Name(), state.Zone)
	segs := dns.SplitDomainName(base)

	r.zone = state.Zone

	if state.QType() == dns.TypeNS {
		return r, nil
	}

	if state.QType() == dns.TypeA && isDefaultNS(state.Name(), r) {
		return r, nil
	}

	offset := 0
	if state.QType() == dns.TypeSRV {
		// The kubernetes peer-finder expects queries with empty port and service to resolve
		// If neither is specified, treat it as a wildcard
		if len(segs) == 3 {
			r.port = "*"
			r.service = "*"
			offset = 0
		} else {
			if len(segs) != 5 {
				return r, errInvalidRequest
			}
			// This is a SRV style request, get first two elements as port and
			// protocol, stripping leading underscores if present.
			if segs[0][0] == '_' {
				r.port = segs[0][1:]
			} else {
				r.port = segs[0]
				if !wildcard(r.port) {
					return r, errInvalidRequest
				}
			}
			if segs[1][0] == '_' {
				r.protocol = segs[1][1:]
				if r.protocol != "tcp" && r.protocol != "udp" {
					return r, errInvalidRequest
				}
			} else {
				r.protocol = segs[1]
				if !wildcard(r.protocol) {
					return r, errInvalidRequest
				}
			}
			if r.port == "" || r.protocol == "" {
				return r, errInvalidRequest
			}
			offset = 2
		}
	}
	if (state.QType() == dns.TypeA || state.QType() == dns.TypeAAAA) && len(segs) == 4 {
		// This is an endpoint A/AAAA record request. Get first element as endpoint.
		r.endpoint = segs[0]
		offset = 1
	}

	if len(segs) == (offset + 3) {
		r.service = segs[offset]
		r.namespace = segs[offset+1]
		r.podOrSvc = segs[offset+2]

		return r, nil
	}

	return r, errInvalidRequest
}

// String return a string representation of r, it just returns all
// fields concatenated with dots.
// This is mostly used in tests.
func (r recordRequest) String() string {
	s := r.port
	s += "." + r.protocol
	s += "." + r.endpoint
	s += "." + r.service
	s += "." + r.namespace
	s += "." + r.podOrSvc
	s += "." + r.zone
	return s
}
