package url

import (
	"fmt"
	"net/url"
)

type (
	URL              = url.URL
	Userinfo         = url.Userinfo
	Values           = url.Values
	Error            = url.Error
	EscapeError      = url.EscapeError
	InvalidHostError = url.InvalidHostError
)

var (
	Parse           = url.Parse
	ParseQuery      = url.ParseQuery
	ParseRequestURI = url.ParseRequestURI
	PathEscape      = url.PathEscape
	PathUnescape    = url.PathUnescape
	QueryEscape     = url.QueryEscape
	QueryUnescape   = url.QueryUnescape
	JoinPath        = url.JoinPath
	User            = url.User
	UserPassword    = url.UserPassword
)

func SetNonZero[T comparable](q Values, key string, value T) {
	var zero T
	if value != zero {
		q.Set(key, fmt.Sprint(value))
	}
}
