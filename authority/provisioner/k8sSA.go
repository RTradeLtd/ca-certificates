package provisioner

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"

	"github.com/RTradeLtd/ca-cli/crypto/pemutil"
	"github.com/RTradeLtd/ca-cli/jose"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ed25519"
)

// NOTE: There can be at most one kubernetes service account provisioner configured
// per instance of step-ca. This is due to a lack of distinguishing information
// contained in kubernetes service account tokens.

const (
	// K8sSAName is the default name used for kubernetes service account provisioners.
	K8sSAName = "k8sSA-default"
	// K8sSAID is the default ID for kubernetes service account provisioners.
	K8sSAID     = "k8ssa/" + K8sSAName
	k8sSAIssuer = "kubernetes/serviceaccount"
)

// This number must <= 1. We'll verify this in Init() below.
var numK8sSAProvisioners = 0

// jwtPayload extends jwt.Claims with step attributes.
type k8sSAPayload struct {
	jose.Claims
	Namespace          string `json:"kubernetes.io/serviceaccount/namespace,omitempty"`
	SecretName         string `json:"kubernetes.io/serviceaccount/secret.name,omitempty"`
	ServiceAccountName string `json:"kubernetes.io/serviceaccount/service-account.name,omitempty"`
	ServiceAccountUID  string `json:"kubernetes.io/serviceaccount/service-account.uid,omitempty"`
}

// K8sSA represents a Kubernetes ServiceAccount provisioner; an
// entity trusted to make signature requests.
type K8sSA struct {
	Type      string  `json:"type"`
	Name      string  `json:"name"`
	Claims    *Claims `json:"claims,omitempty"`
	PubKeys   []byte  `json:"publicKeys,omitempty"`
	claimer   *Claimer
	audiences Audiences
	//kauthn    kauthn.AuthenticationV1Interface
	pubKeys []interface{}
}

// GetID returns the provisioner unique identifier. The name and credential id
// should uniquely identify any K8sSA provisioner.
func (p *K8sSA) GetID() string {
	return K8sSAID
}

// GetTokenID returns an unimplemented error and does not use the input ott.
func (p *K8sSA) GetTokenID(ott string) (string, error) {
	return "", errors.New("not implemented")
}

// GetName returns the name of the provisioner.
func (p *K8sSA) GetName() string {
	return p.Name
}

// GetType returns the type of provisioner.
func (p *K8sSA) GetType() Type {
	return TypeK8sSA
}

// GetEncryptedKey returns false, because the kubernetes provisioner does not
// have access to the private key.
func (p *K8sSA) GetEncryptedKey() (string, string, bool) {
	return "", "", false
}

// Init initializes and validates the fields of a K8sSA type.
func (p *K8sSA) Init(config Config) (err error) {
	switch {
	case p.Type == "":
		return errors.New("provisioner type cannot be empty")
	case p.Name == "":
		return errors.New("provisioner name cannot be empty")
	case numK8sSAProvisioners >= 1:
		return errors.New("cannot have more than one kubernetes service account provisioner")
	}

	if p.PubKeys != nil {
		var (
			block *pem.Block
			rest  = p.PubKeys
		)
		for rest != nil {
			block, rest = pem.Decode(rest)
			if block == nil {
				break
			}
			key, err := pemutil.ParseKey(pem.EncodeToMemory(block))
			if err != nil {
				return errors.Wrapf(err, "error parsing public key in provisioner %s", p.GetID())
			}
			switch q := key.(type) {
			case *rsa.PublicKey, *ecdsa.PublicKey, ed25519.PublicKey:
			default:
				return errors.Errorf("Unexpected public key type %T in provisioner %s", q, p.GetID())
			}
			p.pubKeys = append(p.pubKeys, key)
		}
	} else {
		// TODO: Use the TokenReview API if no pub keys provided. This will need to
		// be configured with additional attributes in the K8sSA struct for
		// connecting to the kubernetes API server.
		return errors.New("K8s Service Account provisioner cannot be initialized without pub keys")
	}
	/*
		// NOTE: Not sure if we should be doing this initialization here ...
		// If you have a k8sSA provisioner defined in your config, but you're not
		// in a kubernetes pod then your CA will fail to startup. Maybe we just postpone
		// creating the authn until token validation time?
		if err := checkAccess(k8s.AuthorizationV1()); err != nil {
			return errors.Wrapf(err, "error verifying access to kubernetes authz service for provisioner %s", p.GetID())
		}

		p.kauthn = k8s.AuthenticationV1()
	*/

	// Update claims with global ones
	if p.claimer, err = NewClaimer(p.Claims, config.Claims); err != nil {
		return err
	}

	p.audiences = config.Audiences
	numK8sSAProvisioners++
	return err
}

// authorizeToken performs common jwt authorization actions and returns the
// claims for case specific downstream parsing.
// e.g. a Sign request will auth/validate different fields than a Revoke request.
func (p *K8sSA) authorizeToken(token string, audiences []string) (*k8sSAPayload, error) {
	jwt, err := jose.ParseSigned(token)
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing token")
	}

	var (
		valid  bool
		claims k8sSAPayload
	)
	if p.pubKeys == nil {
		return nil, errors.New("TokenReview API integration not implemented")
		/* NOTE: We plan to support the TokenReview API in a future release.
		         Below is some code that should be useful when we prioritize
				 this integration.

			tr := kauthnApi.TokenReview{Spec: kauthnApi.TokenReviewSpec{Token: string(token)}}
			rvw, err := p.kauthn.TokenReviews().Create(&tr)
			if err != nil {
				return nil, errors.Wrap(err, "error using kubernetes TokenReview API")
			}
			if rvw.Status.Error != "" {
				return nil, errors.Errorf("error from kubernetes TokenReviewAPI: %s", rvw.Status.Error)
			}
			if !rvw.Status.Authenticated {
				return nil, errors.New("error from kubernetes TokenReviewAPI: token could not be authenticated")
			}
			if err = jwt.UnsafeClaimsWithoutVerification(&claims); err != nil {
				return nil, errors.Wrap(err, "error parsing claims")
			}
		*/
	}
	for _, pk := range p.pubKeys {
		if err = jwt.Claims(pk, &claims); err == nil {
			valid = true
			break
		}
	}
	if !valid {
		return nil, errors.New("error validating token and extracting claims")
	}

	// According to "rfc7519 JSON Web Token" acceptable skew should be no
	// more than a few minutes.
	if err = claims.Validate(jose.Expected{
		Issuer: k8sSAIssuer,
	}); err != nil {
		return nil, errors.Wrapf(err, "invalid token claims")
	}

	if claims.Subject == "" {
		return nil, errors.New("token subject cannot be empty")
	}

	return &claims, nil
}

// AuthorizeRevoke returns an error if the provisioner does not have rights to
// revoke the certificate with serial number in the `sub` property.
func (p *K8sSA) AuthorizeRevoke(token string) error {
	_, err := p.authorizeToken(token, p.audiences.Revoke)
	return err
}

// AuthorizeSign validates the given token.
func (p *K8sSA) AuthorizeSign(ctx context.Context, token string) ([]SignOption, error) {
	_, err := p.authorizeToken(token, p.audiences.Sign)
	if err != nil {
		return nil, err
	}

	// Check for SSH sign-ing request.
	if MethodFromContext(ctx) == SignSSHMethod {
		return nil, errors.New("ssh certificates not enabled for k8s ServiceAccount provisioners")
	}

	return []SignOption{
		// modifiers / withOptions
		newProvisionerExtensionOption(TypeK8sSA, p.Name, ""),
		profileDefaultDuration(p.claimer.DefaultTLSCertDuration()),
		// validators
		defaultPublicKeyValidator{},
		newValidityValidator(p.claimer.MinTLSCertDuration(), p.claimer.MaxTLSCertDuration()),
	}, nil
}

// AuthorizeRenewal returns an error if the renewal is disabled.
func (p *K8sSA) AuthorizeRenewal(cert *x509.Certificate) error {
	if p.claimer.IsDisableRenewal() {
		return errors.Errorf("renew is disabled for provisioner %s", p.GetID())
	}
	return nil
}

/*
func checkAccess(authz kauthz.AuthorizationV1Interface) error {
	r := &kauthzApi.SelfSubjectAccessReview{
		Spec: kauthzApi.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &kauthzApi.ResourceAttributes{
				Group:    "authentication.k8s.io",
				Version:  "v1",
				Resource: "tokenreviews",
				Verb:     "create",
			},
		},
	}
	rvw, err := authz.SelfSubjectAccessReviews().Create(r)
	if err != nil {
		return err
	}
	if !rvw.Status.Allowed {
		return fmt.Errorf("Unable to create kubernetes token reviews: %s", rvw.Status.Reason)
	}

	return nil
}
*/
