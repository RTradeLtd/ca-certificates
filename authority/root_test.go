package authority

import (
	"crypto/x509"
	"net/http"
	"reflect"
	"testing"

	"github.com/RTradeLtd/ca-cli/crypto/pemutil"
	"github.com/pkg/errors"
	"github.com/smallstep/assert"
)

func TestRoot(t *testing.T) {
	a := testAuthority(t)
	a.certificates.Store("invaliddata", "a string") // invalid cert for testing

	tests := map[string]struct {
		sum string
		err *apiError
	}{
		"not-found":                  {"foo", &apiError{errors.New("certificate with fingerprint foo was not found"), http.StatusNotFound, apiCtx{}}},
		"invalid-stored-certificate": {"invaliddata", &apiError{errors.New("stored value is not a *x509.Certificate"), http.StatusInternalServerError, apiCtx{}}},
		"success":                    {"189f573cfa159251e445530847ef80b1b62a3a380ee670dcb49e33ed34da0616", nil},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			crt, err := a.Root(tc.sum)
			if err != nil {
				if assert.NotNil(t, tc.err) {
					switch v := err.(type) {
					case *apiError:
						assert.HasPrefix(t, v.err.Error(), tc.err.Error())
						assert.Equals(t, v.code, tc.err.code)
						assert.Equals(t, v.context, tc.err.context)
					default:
						t.Errorf("unexpected error type: %T", v)
					}
				}
			} else {
				if assert.Nil(t, tc.err) {
					assert.Equals(t, crt, a.rootX509Certs[0])
				}
			}
		})
	}
}

func TestAuthority_GetRootCertificate(t *testing.T) {
	cert, err := pemutil.ReadCertificate("testdata/certs/root_ca.crt")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		want *x509.Certificate
	}{
		{"ok", cert},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := testAuthority(t)
			if got := a.GetRootCertificate(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Authority.GetRootCertificate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthority_GetRootCertificates(t *testing.T) {
	cert, err := pemutil.ReadCertificate("testdata/certs/root_ca.crt")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		want []*x509.Certificate
	}{
		{"ok", []*x509.Certificate{cert}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := testAuthority(t)
			if got := a.GetRootCertificates(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Authority.GetRootCertificates() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthority_GetRoots(t *testing.T) {
	cert, err := pemutil.ReadCertificate("testdata/certs/root_ca.crt")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		want    []*x509.Certificate
		wantErr bool
	}{
		{"ok", []*x509.Certificate{cert}, false},
	}
	for _, tt := range tests {
		a := testAuthority(t)
		t.Run(tt.name, func(t *testing.T) {
			got, err := a.GetRoots()
			if (err != nil) != tt.wantErr {
				t.Errorf("Authority.GetRoots() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Authority.GetRoots() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthority_GetFederation(t *testing.T) {
	cert, err := pemutil.ReadCertificate("testdata/certs/root_ca.crt")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name           string
		wantFederation []*x509.Certificate
		wantErr        bool
		fn             func(a *Authority)
	}{
		{"ok", []*x509.Certificate{cert}, false, nil},
		{"fail", nil, true, func(a *Authority) {
			a.certificates.Store("foo", "bar")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := testAuthority(t)
			if tt.fn != nil {
				tt.fn(a)
			}
			gotFederation, err := a.GetFederation()
			if (err != nil) != tt.wantErr {
				t.Errorf("Authority.GetFederation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotFederation, tt.wantFederation) {
				t.Errorf("Authority.GetFederation() = %v, want %v", gotFederation, tt.wantFederation)
			}
		})
	}
}
