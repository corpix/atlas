package rpc

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"git.tatikoma.dev/corpix/atlas/errors"
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
	var value any
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

func ConvertFromPGToNumericString(value pgtype.Numeric) (string, error) {
	if !value.Valid {
		return "", nil
	}

	raw, err := value.Value()
	if err != nil {
		return "", errors.Wrap(err, "failed to convert numeric to string")
	}

	normalized, ok := raw.(string)
	if !ok {
		return "", errors.Errorf("unexpected numeric value type %T", raw)
	}

	return normalized, nil
}

func ConvertFromNumericStringToPG(value string) (pgtype.Numeric, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return pgtype.Numeric{}, nil
	}

	var (
		numeric pgtype.Numeric
		err     = numeric.Scan(normalized)
	)
	if err != nil {
		return pgtype.Numeric{}, errors.Wrap(err, "failed to parse numeric")
	}
	if !numeric.Valid {
		return pgtype.Numeric{}, errors.New("numeric value is invalid")
	}

	return numeric, nil
}

func ConvertFromDecimalToString(value decimal.Decimal) (string, error) {
	return value.String(), nil
}

func ConvertFromOptionalDecimalToString(value *decimal.Decimal) (string, error) {
	if value == nil {
		return "", nil
	}
	return value.String(), nil
}

func ConvertFromNullDecimalToString(value decimal.NullDecimal) (string, error) {
	if !value.Valid {
		return "", nil
	}
	return value.Decimal.String(), nil
}

func ConvertFromStringToDecimal(value string) (decimal.Decimal, error) {
	if value == "" {
		return decimal.Decimal{}, errors.New("decimal value is empty")
	}

	parsed, err := decimal.NewFromString(value)
	if err != nil {
		return decimal.Decimal{}, errors.Wrap(err, "failed to parse decimal")
	}
	return parsed, nil
}

func ConvertOptionalStringToDecimal(value string) (*decimal.Decimal, error) {
	if value == "" {
		return nil, nil
	}

	parsed, err := ConvertFromStringToDecimal(value)
	if err != nil {
		return nil, err
	}

	return &parsed, nil
}

func ConvertOptionalStringToNullDecimal(value string) (decimal.NullDecimal, error) {
	if value == "" {
		return decimal.NullDecimal{}, nil
	}

	parsed, err := ConvertFromStringToDecimal(value)
	if err != nil {
		return decimal.NullDecimal{}, err
	}

	return decimal.NullDecimal{Decimal: parsed, Valid: true}, nil
}

func NormalizeNumericString(value string) (string, error) {
	numeric, err := ConvertFromNumericStringToPG(value)
	if err != nil {
		return "", err
	}
	if !numeric.Valid {
		return "", errors.New("numeric value is empty")
	}

	raw, err := numeric.Value()
	if err != nil {
		return "", errors.Wrap(err, "failed to normalize numeric")
	}

	normalized, ok := raw.(string)
	if !ok {
		return "", errors.Errorf("unexpected numeric value type %T", raw)
	}

	return normalized, nil
}

func ConvertOptionalNumericStringToPG(value string) (*pgtype.Numeric, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return nil, nil
	}

	numeric, err := ConvertFromNumericStringToPG(normalized)
	if err != nil {
		return nil, err
	}
	if !numeric.Valid {
		return nil, errors.New("numeric value is invalid")
	}

	return &numeric, nil
}

func ConvertFromAnyToPGNumeric(value any) (pgtype.Numeric, error) {
	switch normalized := value.(type) {
	case pgtype.Numeric:
		return normalized, nil
	case *pgtype.Numeric:
		if normalized == nil {
			return pgtype.Numeric{}, nil
		}
		return *normalized, nil
	case string:
		var (
			numeric pgtype.Numeric
			err     = numeric.Scan(normalized)
		)
		if err != nil {
			return pgtype.Numeric{}, errors.Wrap(err, "failed to parse numeric")
		}
		return numeric, nil
	default:
		return pgtype.Numeric{}, errors.Errorf("unsupported numeric type %T", value)
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
	for n := range 32 {
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
