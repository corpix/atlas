package rpc

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func ConvertMapToJSON(m map[string]*structpb.Value) ([]byte, error) {
	return protojson.Marshal(&structpb.Struct{Fields: m})
}

func ConvertJSONToMap(j []byte) (map[string]*structpb.Value, error) {
	s := &structpb.Struct{}
	err := protojson.Unmarshal(j, s)
	if err != nil {
		return nil, err
	}
	return s.Fields, nil
}

func ConvertJSONToStruct(j []byte) (*structpb.Value, error) {
	var value interface{}
	err := json.Unmarshal(j, &value)
	if err != nil {
		return nil, err
	}
	return structpb.NewValue(value)
}

func ConvertFromPGToPBTimestamp(t pgtype.Timestamptz) *timestamppb.Timestamp {
	if !t.Valid {
		return nil
	}
	return timestamppb.New(t.Time)
}

func ConvertFromPBToPGTimestamp(t *timestamppb.Timestamp) pgtype.Timestamptz {
	return pgtype.Timestamptz{
		Time:             t.AsTime(),
		InfinityModifier: pgtype.Finite,
		Valid:            true,
	}
}

func ConvertCIDR(cidrStr string) (netip.Prefix, error) {
	cidr, err := netip.ParsePrefix(cidrStr)
	if err != nil {
		return cidr, err
	}
	return cidr, nil
}

func ConvertCIDRLoose(ipStr string) []netip.Prefix {
	ipStr = strings.TrimSpace(ipStr)

	res := []netip.Prefix{}
	cidr, err := netip.ParsePrefix(ipStr)
	if err == nil {
		res = append(res, cidr)
	}
	ip, err := netip.ParseAddr(ipStr)
	if err == nil {
		res = append(res, netip.PrefixFrom(ip, ip.BitLen()))
	}
	cidr, err = convertCIDRLoosePartialIPv4(ipStr)
	if err == nil {
		res = append(res, cidr)
	}
	cidr, err = convertCIDRLoosePartialIPv6(ipStr)
	if err == nil {
		res = append(res, cidr)
	}
	return res
}

func convertCIDRLoosePartialIPv4(ipStr string) (netip.Prefix, error) {
	ipStr = strings.Trim(ipStr, ".")
	parts := strings.Split(ipStr, ".")
	if len(parts) > 4 {
		return netip.Prefix{}, fmt.Errorf("invalid IPv4 address format")
	}
	mask := len(parts) * 8
	for len(parts) < 4 {
		parts = append(parts, "0")
	}
	fullIP := strings.Join(parts, ".")
	cidrNotation := fullIP + "/" + strconv.Itoa(mask)
	return netip.ParsePrefix(cidrNotation)
}

func convertCIDRLoosePartialIPv6(ipStr string) (netip.Prefix, error) {
	ipStr = strings.Trim(ipStr, "[]")
	ipStr = strings.ReplaceAll(ipStr, ":", "")

	ip := ""
	mask := len(ipStr) * 4
	for n := 0; n < 32; n++ {
		if n < len(ipStr) {
			ip += string(ipStr[n])
		} else {
			ip += "0"
		}
		if n+1 != 32 && (n+1)%4 == 0 {
			ip += ":"
		}
	}
	return netip.ParsePrefix(ip + "/" + strconv.Itoa(mask))
}
