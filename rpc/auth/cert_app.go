package auth

import (
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"git.tatikoma.dev/corpix/atlas/app"
	"git.tatikoma.dev/corpix/atlas/errors"
	"git.tatikoma.dev/corpix/atlas/log"
)

type (
	CertApp struct {
		Registry           *CertTypeRegistry
		setGenerateOptions func(*app.Context, *CertToolGenerateOptions) error
	}
	CertAppOption func(*CertApp)
)

const (
	CertRevocationReasonUnspecified          = 0
	CertRevocationReasonKeyCompromise        = 1
	CertRevocationReasonCACompromise         = 2
	CertRevocationReasonAffiliationChanged   = 3
	CertRevocationReasonSuperseded           = 4
	CertRevocationReasonCessationOfOperation = 5
	CertRevocationReasonCertificateHold      = 6
	CertRevocationReasonRemoveFromCRL        = 8
	CertRevocationReasonPrivilegeWithdrawn   = 9
	CertRevocationReasonAACompromise         = 10
)

var (
	CertRevocationReasons = map[int]string{
		CertRevocationReasonUnspecified:          "unspecified",
		CertRevocationReasonKeyCompromise:        "key_compromise",
		CertRevocationReasonCACompromise:         "ca_compromise",
		CertRevocationReasonAffiliationChanged:   "affiliation_changed",
		CertRevocationReasonSuperseded:           "superseded",
		CertRevocationReasonCessationOfOperation: "cessation_of_operation",
		CertRevocationReasonCertificateHold:      "certificate_hold",
		CertRevocationReasonRemoveFromCRL:        "remove_from_crl",
		CertRevocationReasonPrivilegeWithdrawn:   "privilege_withdrawn",
		CertRevocationReasonAACompromise:         "aa_compromise",
	}

	CertRevocationReasonsByName = map[string]int{}
)

func NewCertApp(opts ...CertAppOption) *CertApp {
	app := &CertApp{}
	for _, opt := range opts {
		opt(app)
	}
	return app
}

func WithCertAppRegistry(registry *CertTypeRegistry) CertAppOption {
	return func(app *CertApp) {
		app.Registry = registry
	}
}

func WithCertAppGenerateOptions(
	setter func(*app.Context, *CertToolGenerateOptions) error,
) CertAppOption {
	return func(app *CertApp) {
		app.setGenerateOptions = setter
	}
}

func init() {
	for reason, name := range CertRevocationReasons {
		CertRevocationReasonsByName[name] = reason
	}
}

func (*CertApp) Flags() app.Flags {
	return app.Flags{
		&app.StringFlag{
			Name:  "name",
			Usage: "name to prepend to output certificate (eg %name%.ca-cert.pem)",
		},
		&app.StringFlag{
			Name:  "type",
			Usage: "type of certificate to generate",
		},
		&app.BoolFlag{
			Name:  "generate-ca",
			Usage: "generate CA certificate and key",
		},
		&app.BoolFlag{
			Name:  "revoke",
			Usage: "revoke certificate and update CRL",
		},
		&app.BoolFlag{
			Name:  "init-crl",
			Usage: "initialize CRL file if missing",
		},
		&app.StringFlag{
			Name:  "ca-cert",
			Usage: "path to CA certificate (defaults to ./ca-cert.pem or name prefix)",
		},
		&app.StringFlag{
			Name:  "ca-key",
			Usage: "path to CA key (defaults to ./ca-key.pem or name prefix)",
		},
		&app.StringFlag{
			Name:  "crl",
			Usage: "path to CRL file (defaults to ./ca-crl.pem or name prefix)",
		},
		&app.StringFlag{
			Name:  "cert-path",
			Usage: "path to certificate to revoke",
		},
		&app.StringFlag{
			Name:  "serial",
			Usage: "serial number to revoke (decimal or 0x prefixed)",
		},
		&app.StringFlag{
			Name:  "mode",
			Usage: "file mode for generated files (octal, e.g. 640)",
			Value: "640",
		},
		&app.StringFlag{
			Name:  "revocation-time",
			Usage: "revocation time (RFC3339, defaults to now)",
		},
		&app.StringFlag{
			Name:  "revocation-reason",
			Usage: "CRL revocation reason",
		},
		&app.DurationFlag{
			Name:  "crl-validity",
			Usage: "CRL validity duration (e.g. 168h)",
		},
		&app.StringFlag{
			Name:  "ip-addresses",
			Usage: "comma separated list of allowed ip addresses to encode into certificate",
			Value: "127.0.0.1",
		},
		&app.StringFlag{
			Name:  "dns-names",
			Usage: "comma separated list of allowed hostnames to encode into certificate",
		},
		&app.StringFlag{
			Name:  "common-name",
			Usage: "common name for certificate",
			Value: "localhost",
		},
		&app.StringFlag{
			Name:  "region",
			Usage: "region identifier to encode into certificate subject",
		},
	}
}

func (a *CertApp) Command() *app.Command {
	return &app.Command{
		Name:   "cert",
		Action: a.Cert,
		Flags:  a.Flags(),
	}
}

func (a *CertApp) Cert(ctx *app.Context) error {
	generateCA := ctx.Bool("generate-ca")
	revoke := ctx.Bool("revoke")
	initCRL := ctx.Bool("init-crl")
	certType := ctx.String("type")
	fileMode, err := parseFileMode(ctx.String("mode"))
	if err != nil {
		return err
	}

	if revoke && initCRL {
		return errors.New("init-crl and revoke are mutually exclusive")
	}
	if !(generateCA || revoke || initCRL) && certType == "" {
		return errors.New("certificate type is required")
	}

	tool := NewCertTool(a.Registry)
	if generateCA {
		err := tool.Generate(CertToolGenerateOptions{
			NamePrefix: ctx.String("name"),
			CACertPath: ctx.String("ca-cert"),
			CAKeyPath:  ctx.String("ca-key"),
			CommonName: ctx.String("common-name"),
			Region:     ctx.String("region"),
			FileMode:   fileMode,
			GenerateCA: true,
		})
		if err != nil {
			return errors.Wrap(err, "error generating CA certificates")
		}
		log.Info().Msg("generated CA certificate")
	}

	if initCRL {
		err := tool.InitCRL(CertToolCRLInitOptions{
			NamePrefix:  ctx.String("name"),
			CACertPath:  ctx.String("ca-cert"),
			CAKeyPath:   ctx.String("ca-key"),
			CRLPath:     ctx.String("crl"),
			CRLValidity: ctx.Duration("crl-validity"),
			FileMode:    fileMode,
		})
		if err != nil {
			return errors.Wrap(err, "error initializing CRL")
		}
		log.Info().Msg("initialized CRL")
	}

	if revoke {
		var (
			revocationTime time.Time
			crlValidity    time.Duration
			err            error
		)
		revocationTimeText := ctx.String("revocation-time")
		if revocationTimeText != "" {
			revocationTime, err = time.Parse(time.RFC3339, revocationTimeText)
			if err != nil {
				return errors.Wrap(err, "invalid revocation time")
			}
		}

		crlValidity = ctx.Duration("crl-validity")
		if crlValidity < 0 {
			return errors.New("invalid crl validity")
		}

		serial := ctx.String("serial")
		if serial != "" {
			serialNumber := new(big.Int)
			if _, ok := serialNumber.SetString(serial, 0); !ok {
				return errors.Errorf("invalid serial number %q", serial)
			}
		}

		reasonCode, err := a.parseRevocationReason(ctx.String("revocation-reason"))
		if err != nil {
			return err
		}

		err = tool.Revoke(CertToolRevokeOptions{
			NamePrefix:     ctx.String("name"),
			CACertPath:     ctx.String("ca-cert"),
			CAKeyPath:      ctx.String("ca-key"),
			CRLPath:        ctx.String("crl"),
			CertPath:       ctx.String("cert-path"),
			SerialNumber:   serial,
			ReasonCode:     reasonCode,
			RevocationTime: revocationTime,
			CRLValidity:    crlValidity,
			FileMode:       fileMode,
		})
		if err != nil {
			return errors.Wrap(err, "error revoking certificate")
		}
		log.Info().Msg("revoked certificate")
	}

	if certType != "" {
		opts := CertToolGenerateOptions{
			NamePrefix:  ctx.String("name"),
			Type:        certType,
			CACertPath:  ctx.String("ca-cert"),
			CAKeyPath:   ctx.String("ca-key"),
			FileMode:    fileMode,
			IPAddresses: ctx.String("ip-addresses"),
			DNSNames:    ctx.String("dns-names"),
			CommonName:  ctx.String("common-name"),
			Region:      ctx.String("region"),
		}
		if a.setGenerateOptions != nil {
			err := a.setGenerateOptions(ctx, &opts)
			if err != nil {
				return err
			}
		}

		err := tool.Generate(opts)
		if err != nil {
			return errors.Wrap(err, "error generating certificates")
		}
		log.Info().Msg("generated certificate")
	}

	return nil
}

func (*CertApp) parseRevocationReason(reason string) (int, error) {
	if reason == "" {
		return CertRevocationReasonUnspecified, nil
	}
	if code, ok := CertRevocationReasonsByName[reason]; ok {
		return code, nil
	}

	return 0, errors.Errorf("invalid revocation reason: %s", reason)
}

func parseFileMode(text string) (os.FileMode, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, nil
	}
	value, err := strconv.ParseUint(text, 8, 32)
	if err != nil {
		return 0, errors.Wrapf(err, "invalid mode %q", text)
	}
	return os.FileMode(value), nil
}
