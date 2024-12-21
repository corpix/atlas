package auth

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type (
	Capability struct {
		ID         CapabilityLiteral
		Parameters []string
	}
	Capabilities map[CapabilityLiteral]*Capability
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

func (c Capabilities) Match(wantCapsIds ...CapabilityLiteral) Capabilities {
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

func (c Capabilities) Get(wantCapId CapabilityLiteral) *Capability {
	currentCap, ok := c[wantCapId]
	if !ok {
		return nil
	}
	return currentCap
}

func (c Capabilities) Assert(wantCapId CapabilityLiteral) *Capability {
	cap := c.Get(wantCapId)
	if cap == nil {
		panic(fmt.Errorf(
			"capability %q is not satisfied",
			wantCapId,
		))
	}
	return cap
}

func NewCapability(id CapabilityLiteral, params ...string) *Capability {
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
	CapabilityRuleAnd []CapabilityLiteral
	CapabilityRuleOr  []CapabilityLiteral

	// CapabilityLiteral contains (uniq) name of capability
	CapabilityLiteral  string
	CapabilityLiterals []CapabilityLiteral
)

func (cid CapabilityLiteral) String() string {
	return string(cid)
}

func (cid CapabilityLiteral) Match(caps Capabilities) bool {
	_, ok := caps[cid]
	return ok
}

func (c CapabilityLiterals) String() string {
	names := make([]string, 0, len(c))
	for _, v := range c {
		names = append(names, string(v))
	}
	sort.Strings(names)
	return "[" + strings.Join(names, ", ") + "]"
}

func CapRuleAnd(ids ...CapabilityLiteral) CapabilityRuleAnd {
	return CapabilityRuleAnd(ids)
}
func CapRuleOr(ids ...CapabilityLiteral) CapabilityRuleOr {
	return CapabilityRuleOr(ids)
}

func (cr CapabilityRuleAnd) String() string {
	return CapabilityLiterals(cr).String()
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
	return CapabilityLiterals(cr).String()
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
