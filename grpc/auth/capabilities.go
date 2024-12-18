package auth

import (
	"context"
	"encoding/asn1"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CapabilityID contains (uniq) name of capability
type (
	CapabilityID  string
	CapabilityIDs []CapabilityID
)

var (
	// private_prefix + [ord(x) for x in "rforge"]
	CapabilitiesCertificateOID = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 114, 102, 111, 114, 103, 101}

	capabilityIds      = map[CapabilityID]void{}
	CapabilityIDAgent  = defineCapability("agent")
	CapabilityIDAdmin  = defineCapability("admin")
	CapabilityIDReader = defineCapability("reader")
)

func (cid CapabilityID) String() string {
	return string(cid)
}

func (cid CapabilityID) Match(caps Capabilities) bool {
	_, ok := caps[cid]
	return ok
}

func (c CapabilityIDs) String() string {
	names := make([]string, 0, len(c))
	for _, v := range c {
		names = append(names, string(v))
	}
	sort.Strings(names)
	return "[" + strings.Join(names, ", ") + "]"
}

func defineCapability(name string) CapabilityID {
	id := CapabilityID(name)
	capabilityIds[id] = void{}
	return id
}

//

type (
	Capability struct {
		ID         CapabilityID
		Parameters []string
	}
	Capabilities map[CapabilityID]*Capability
)

func (c *Capability) String() string {
	str := string(c.ID)
	for _, parameter := range c.Parameters {
		str += ":" + parameter
	}
	return str
}

func (c Capabilities) String() string {
	names := make([]string, 0, len(c))
	for _, v := range c {
		names = append(names, v.String())
	}
	sort.Strings(names)
	return "[" + strings.Join(names, ", ") + "]"
}

func (c Capabilities) Match(wantCapsIds ...CapabilityID) Capabilities {
	matchedCaps := make(Capabilities, len(wantCapsIds))
	for _, capId := range wantCapsIds {
		for _, currentCap := range c {
			if capId == currentCap.ID {
				matchedCaps[capId] = currentCap
				break
			}
		}
	}
	return matchedCaps
}

func (c Capabilities) Get(wantCapId CapabilityID) *Capability {
	currentCap, ok := c[wantCapId]
	if !ok {
		return nil
	}
	return currentCap
}

func (c Capabilities) Assert(wantCapId CapabilityID) *Capability {
	cap := c.Get(wantCapId)
	if cap == nil {
		panic(fmt.Errorf(
			"capability %q is not satisfied",
			wantCapId,
		))
	}
	return cap
}

func NewCapability(id CapabilityID, params ...string) *Capability {
	return &Capability{
		ID:         id,
		Parameters: params,
	}
}

func CapabilitiesFromContext(ctx context.Context) Capabilities {
	capsValue := ctx.Value(AuthCapabilitiesContextKey)
	caps, ok := capsValue.(Capabilities)
	if !ok {
		return nil
	}
	return caps
}

//

type (
	CapabilityRule interface {
		String() string
		Match(Capabilities) bool
	}
	CapabilityRuleAnd []CapabilityID
	CapabilityRuleOr  []CapabilityID
)

func CapRuleAnd(ids ...CapabilityID) CapabilityRuleAnd {
	return CapabilityRuleAnd(ids)
}
func CapRuleOr(ids ...CapabilityID) CapabilityRuleOr {
	return CapabilityRuleOr(ids)
}

func (cr CapabilityRuleAnd) String() string {
	return CapabilityIDs(cr).String()
}
func (cr CapabilityRuleAnd) Match(caps Capabilities) bool {
	var ok bool
	for _, cap := range cr {
		_, ok = caps[cap]
		if !ok {
			return false
		}
	}
	return true
}

func (cr CapabilityRuleOr) String() string {
	return CapabilityIDs(cr).String()
}
func (cr CapabilityRuleOr) Match(caps Capabilities) bool {
	var ok bool
	for _, cap := range cr {
		_, ok = caps[cap]
		if ok {
			return true
		}
	}
	return false
}

func CapabilitiesAssert(caps Capabilities, rule CapabilityRule) (Capabilities, error) {
	if !rule.Match(caps) {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"required client capability set not satisfied, has: %s, want: %s",
			caps.String(), rule.String(),
		)
	}
	return caps, nil
}

//

type CapabilityRuleMap map[string]CapabilityRule

func (cr CapabilityRuleMap) Match(caps Capabilities, method string) (CapabilityRule, bool) {
	rule, ok := cr[method]
	if !ok {
		// no rules, assuming public api
		return rule, true
	}
	return rule, rule.Match(caps)
}
