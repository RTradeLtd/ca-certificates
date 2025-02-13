package provisioner

import (
	"testing"
	"time"

	"github.com/RTradeLtd/ca-cli/crypto/keys"
	"github.com/pkg/errors"
	"github.com/smallstep/assert"
	"golang.org/x/crypto/ssh"
)

func Test_sshCertificateDefaultValidator_Valid(t *testing.T) {
	pub, _, err := keys.GenerateDefaultKeyPair()
	assert.FatalError(t, err)
	sshPub, err := ssh.NewPublicKey(pub)
	assert.FatalError(t, err)
	v := sshCertificateDefaultValidator{}
	tests := []struct {
		name string
		cert *ssh.Certificate
		err  error
	}{
		{
			"fail/zero-nonce",
			&ssh.Certificate{},
			errors.New("ssh certificate nonce cannot be empty"),
		},
		{
			"fail/nil-key",
			&ssh.Certificate{Nonce: []byte("foo")},
			errors.New("ssh certificate key cannot be nil"),
		},
		{
			"fail/zero-serial",
			&ssh.Certificate{Nonce: []byte("foo"), Key: sshPub},
			errors.New("ssh certificate serial cannot be 0"),
		},
		{
			"fail/unexpected-cert-type",
			// UserCert = 1, HostCert = 2
			&ssh.Certificate{Nonce: []byte("foo"), Key: sshPub, CertType: 3, Serial: 1},
			errors.New("ssh certificate has an unknown type: 3"),
		},
		{
			"fail/empty-cert-key-id",
			&ssh.Certificate{Nonce: []byte("foo"), Key: sshPub, Serial: 1, CertType: 1},
			errors.New("ssh certificate key id cannot be empty"),
		},
		{
			"fail/empty-valid-principals",
			&ssh.Certificate{Nonce: []byte("foo"), Key: sshPub, Serial: 1, CertType: 1, KeyId: "foo"},
			errors.New("ssh certificate valid principals cannot be empty"),
		},
		{
			"fail/zero-validAfter",
			&ssh.Certificate{
				Nonce:           []byte("foo"),
				Key:             sshPub,
				Serial:          1,
				CertType:        1,
				KeyId:           "foo",
				ValidPrincipals: []string{"foo"},
				ValidAfter:      0,
			},
			errors.New("ssh certificate validAfter cannot be 0"),
		},
		{
			"fail/validBefore-past",
			&ssh.Certificate{
				Nonce:           []byte("foo"),
				Key:             sshPub,
				Serial:          1,
				CertType:        1,
				KeyId:           "foo",
				ValidPrincipals: []string{"foo"},
				ValidAfter:      uint64(time.Now().Add(-10 * time.Minute).Unix()),
				ValidBefore:     uint64(time.Now().Add(-5 * time.Minute).Unix()),
			},
			errors.New("ssh certificate validBefore cannot be in the past"),
		},
		{
			"fail/validAfter-after-validBefore",
			&ssh.Certificate{
				Nonce:           []byte("foo"),
				Key:             sshPub,
				Serial:          1,
				CertType:        1,
				KeyId:           "foo",
				ValidPrincipals: []string{"foo"},
				ValidAfter:      uint64(time.Now().Add(15 * time.Minute).Unix()),
				ValidBefore:     uint64(time.Now().Add(10 * time.Minute).Unix()),
			},
			errors.New("ssh certificate validBefore cannot be before validAfter"),
		},
		{
			"fail/empty-extensions",
			&ssh.Certificate{
				Nonce:           []byte("foo"),
				Key:             sshPub,
				Serial:          1,
				CertType:        1,
				KeyId:           "foo",
				ValidPrincipals: []string{"foo"},
				ValidAfter:      uint64(time.Now().Unix()),
				ValidBefore:     uint64(time.Now().Add(10 * time.Minute).Unix()),
			},
			errors.New("ssh certificate extensions cannot be empty"),
		},
		{
			"fail/nil-signature-key",
			&ssh.Certificate{
				Nonce:           []byte("foo"),
				Key:             sshPub,
				Serial:          1,
				CertType:        1,
				KeyId:           "foo",
				ValidPrincipals: []string{"foo"},
				ValidAfter:      uint64(time.Now().Unix()),
				ValidBefore:     uint64(time.Now().Add(10 * time.Minute).Unix()),
				Permissions: ssh.Permissions{
					Extensions: map[string]string{"foo": "bar"},
				},
			},
			errors.New("ssh certificate signature key cannot be nil"),
		},
		{
			"fail/nil-signature",
			&ssh.Certificate{
				Nonce:           []byte("foo"),
				Key:             sshPub,
				Serial:          1,
				CertType:        1,
				KeyId:           "foo",
				ValidPrincipals: []string{"foo"},
				ValidAfter:      uint64(time.Now().Unix()),
				ValidBefore:     uint64(time.Now().Add(10 * time.Minute).Unix()),
				Permissions: ssh.Permissions{
					Extensions: map[string]string{"foo": "bar"},
				},
				SignatureKey: sshPub,
			},
			errors.New("ssh certificate signature cannot be nil"),
		},
		{
			"ok/userCert",
			&ssh.Certificate{
				Nonce:           []byte("foo"),
				Key:             sshPub,
				Serial:          1,
				CertType:        1,
				KeyId:           "foo",
				ValidPrincipals: []string{"foo"},
				ValidAfter:      uint64(time.Now().Unix()),
				ValidBefore:     uint64(time.Now().Add(10 * time.Minute).Unix()),
				Permissions: ssh.Permissions{
					Extensions: map[string]string{"foo": "bar"},
				},
				SignatureKey: sshPub,
				Signature:    &ssh.Signature{},
			},
			nil,
		},
		{
			"ok/hostCert",
			&ssh.Certificate{
				Nonce:           []byte("foo"),
				Key:             sshPub,
				Serial:          1,
				CertType:        2,
				KeyId:           "foo",
				ValidPrincipals: []string{"foo"},
				ValidAfter:      uint64(time.Now().Unix()),
				ValidBefore:     uint64(time.Now().Add(10 * time.Minute).Unix()),
				SignatureKey:    sshPub,
				Signature:       &ssh.Signature{},
			},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := v.Valid(tt.cert); err != nil {
				if assert.NotNil(t, tt.err) {
					assert.HasPrefix(t, err.Error(), tt.err.Error())
				}
			} else {
				assert.Nil(t, tt.err)
			}
		})
	}
}

func Test_sshCertificateValidityValidator(t *testing.T) {
	p, err := generateX5C(nil)
	assert.FatalError(t, err)
	v := sshCertificateValidityValidator{p.claimer}
	n := now()
	tests := []struct {
		name string
		cert *ssh.Certificate
		err  error
	}{
		{
			"fail/validAfter-0",
			&ssh.Certificate{CertType: ssh.UserCert},
			errors.New("ssh certificate validAfter cannot be 0"),
		},
		{
			"fail/validBefore-in-past",
			&ssh.Certificate{CertType: ssh.UserCert, ValidAfter: uint64(now().Unix()), ValidBefore: uint64(now().Add(-time.Minute).Unix())},
			errors.New("ssh certificate validBefore cannot be in the past"),
		},
		{
			"fail/validBefore-before-validAfter",
			&ssh.Certificate{CertType: ssh.UserCert, ValidAfter: uint64(now().Add(5 * time.Minute).Unix()), ValidBefore: uint64(now().Add(3 * time.Minute).Unix())},
			errors.New("ssh certificate validBefore cannot be before validAfter"),
		},
		{
			"fail/cert-type-not-set",
			&ssh.Certificate{ValidAfter: uint64(now().Unix()), ValidBefore: uint64(now().Add(10 * time.Minute).Unix())},
			errors.New("ssh certificate type has not been set"),
		},
		{
			"fail/unexpected-cert-type",
			&ssh.Certificate{
				CertType:    3,
				ValidAfter:  uint64(now().Unix()),
				ValidBefore: uint64(now().Add(10 * time.Minute).Unix()),
			},
			errors.New("unknown ssh certificate type 3"),
		},
		{
			"fail/duration<min",
			&ssh.Certificate{
				CertType:    1,
				ValidAfter:  uint64(n.Unix()),
				ValidBefore: uint64(n.Add(4 * time.Minute).Unix()),
			},
			errors.New("requested duration of 4m0s is less than minimum accepted duration for selected provisioner of 5m0s"),
		},
		{
			"fail/duration>max",
			&ssh.Certificate{
				CertType:    1,
				ValidAfter:  uint64(n.Unix()),
				ValidBefore: uint64(n.Add(48 * time.Hour).Unix()),
			},
			errors.New("requested duration of 48h0m0s is greater than maximum accepted duration for selected provisioner of 24h0m0s"),
		},
		{
			"ok",
			&ssh.Certificate{
				CertType:    1,
				ValidAfter:  uint64(now().Unix()),
				ValidBefore: uint64(now().Add(8 * time.Hour).Unix()),
			},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := v.Valid(tt.cert); err != nil {
				if assert.NotNil(t, tt.err) {
					assert.HasPrefix(t, err.Error(), tt.err.Error())
				}
			} else {
				assert.Nil(t, tt.err)
			}
		})
	}
}

func Test_sshValidityModifier(t *testing.T) {
	n := now()
	p, err := generateX5C(nil)
	assert.FatalError(t, err)
	type test struct {
		svm   *sshValidityModifier
		cert  *ssh.Certificate
		valid func(*ssh.Certificate)
		err   error
	}
	tests := map[string]func() test{
		"fail/type-not-set": func() test {
			return test{
				svm: &sshValidityModifier{Claimer: p.claimer, validBefore: n.Add(6 * time.Hour)},
				cert: &ssh.Certificate{
					ValidAfter:  uint64(n.Unix()),
					ValidBefore: uint64(n.Add(8 * time.Hour).Unix()),
				},
				err: errors.New("ssh certificate type has not been set"),
			}
		},
		"fail/type-not-recognized": func() test {
			return test{
				svm: &sshValidityModifier{Claimer: p.claimer, validBefore: n.Add(6 * time.Hour)},
				cert: &ssh.Certificate{
					CertType:    4,
					ValidAfter:  uint64(n.Unix()),
					ValidBefore: uint64(n.Add(8 * time.Hour).Unix()),
				},
				err: errors.New("unknown ssh certificate type 4"),
			}
		},
		"fail/requested-validAfter-after-limit": func() test {
			return test{
				svm: &sshValidityModifier{Claimer: p.claimer, validBefore: n.Add(1 * time.Hour)},
				cert: &ssh.Certificate{
					CertType:    1,
					ValidAfter:  uint64(n.Add(2 * time.Hour).Unix()),
					ValidBefore: uint64(n.Add(8 * time.Hour).Unix()),
				},
				err: errors.Errorf("provisioning credential expiration ("),
			}
		},
		"fail/requested-validBefore-after-limit": func() test {
			return test{
				svm: &sshValidityModifier{Claimer: p.claimer, validBefore: n.Add(1 * time.Hour)},
				cert: &ssh.Certificate{
					CertType:    1,
					ValidAfter:  uint64(n.Unix()),
					ValidBefore: uint64(n.Add(2 * time.Hour).Unix()),
				},
				err: errors.New("provisioning credential expiration ("),
			}
		},
		"ok/valid-requested-validBefore": func() test {
			va, vb := uint64(n.Unix()), uint64(n.Add(2*time.Hour).Unix())
			return test{
				svm: &sshValidityModifier{Claimer: p.claimer, validBefore: n.Add(3 * time.Hour)},
				cert: &ssh.Certificate{
					CertType:    1,
					ValidAfter:  va,
					ValidBefore: vb,
				},
				valid: func(cert *ssh.Certificate) {
					assert.Equals(t, cert.ValidAfter, va)
					assert.Equals(t, cert.ValidBefore, vb)
				},
			}
		},
		"ok/empty-requested-validBefore-limit-after-default": func() test {
			va := uint64(n.Unix())
			return test{
				svm: &sshValidityModifier{Claimer: p.claimer, validBefore: n.Add(5 * time.Hour)},
				cert: &ssh.Certificate{
					CertType:   1,
					ValidAfter: va,
				},
				valid: func(cert *ssh.Certificate) {
					assert.Equals(t, cert.ValidAfter, va)
					assert.Equals(t, cert.ValidBefore, uint64(n.Add(4*time.Hour).Unix()))
				},
			}
		},
		"ok/empty-requested-validBefore-limit-before-default": func() test {
			va := uint64(n.Unix())
			return test{
				svm: &sshValidityModifier{Claimer: p.claimer, validBefore: n.Add(3 * time.Hour)},
				cert: &ssh.Certificate{
					CertType:   1,
					ValidAfter: va,
				},
				valid: func(cert *ssh.Certificate) {
					assert.Equals(t, cert.ValidAfter, va)
					assert.Equals(t, cert.ValidBefore, uint64(n.Add(3*time.Hour).Unix()))
				},
			}
		},
	}
	for name, run := range tests {
		t.Run(name, func(t *testing.T) {
			tt := run()
			if err := tt.svm.Modify(tt.cert); err != nil {
				if assert.NotNil(t, tt.err) {
					assert.HasPrefix(t, err.Error(), tt.err.Error())
				}
			} else {
				if assert.Nil(t, tt.err) {
					tt.valid(tt.cert)
				}
			}
		})
	}
}
