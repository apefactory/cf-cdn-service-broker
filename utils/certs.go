package utils

import (
	"path"
	"strings"

	"crypto"
	"crypto/rand"
	"crypto/rsa"

	"github.com/xenolf/lego/acme"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/18F/cf-cdn-service-broker/config"
)

type User struct {
	Email        string
	Registration *acme.RegistrationResource
	key          crypto.PrivateKey
}

func (u *User) GetEmail() string {
	return u.Email
}

func (u *User) GetRegistration() *acme.RegistrationResource {
	return u.Registration
}

func (u *User) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

type HTTPProvider struct {
	Settings config.Settings
	Service  *s3.S3
}

func (p *HTTPProvider) Present(domain, token, keyAuth string) error {
	_, err := p.Service.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(p.Settings.Bucket),
		Body:   strings.NewReader(keyAuth),
		Key:    aws.String(path.Join(".well-known", "acme-challenge", token)),
	})

	return err
}

func (p *HTTPProvider) CleanUp(domain, token, keyAuth string) error {
	_, err := p.Service.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(p.Settings.Bucket),
		Key:    aws.String(path.Join(".well-known", "acme-challenge", token)),
	})

	return err
}

type AcmeIface interface {
	ObtainCertificate(domain string) (acme.CertificateResource, error)
	RenewCertificate(cert acme.CertificateResource) (acme.CertificateResource, error)
}

type Acme struct {
	Settings config.Settings
	Service  *s3.S3
}

func (a *Acme) ObtainCertificate(domain string) (acme.CertificateResource, error) {
	client, err := a.newClient()
	if err != nil {
		return acme.CertificateResource{}, err
	}

	domains := []string{domain}
	certificate, failures := client.ObtainCertificate(domains, true, nil)

	if len(failures) > 0 {
		return acme.CertificateResource{}, failures[domain]
	}

	return certificate, nil
}

func (a *Acme) RenewCertificate(cert acme.CertificateResource) (acme.CertificateResource, error) {
	client, err := a.newClient()
	if err != nil {
		return acme.CertificateResource{}, err
	}

	return client.RenewCertificate(cert, true)
}

func (a *Acme) newClient() (*acme.Client, error) {
	keySize := 2048
	key, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return &acme.Client{}, err
	}

	user := User{
		Email: a.Settings.Email,
		key:   key,
	}

	client, err := acme.NewClient(a.Settings.AcmeUrl, &user, acme.RSA2048)
	if err != nil {
		return &acme.Client{}, err
	}

	reg, err := client.Register()

	if err != nil {
		return &acme.Client{}, err
	}

	user.Registration = reg

	err = client.AgreeToTOS()

	if err != nil {
		return &acme.Client{}, err
	}

	client.SetChallengeProvider(acme.HTTP01, &HTTPProvider{
		Settings: a.Settings,
		Service:  a.Service,
	})
	client.ExcludeChallenges([]acme.Challenge{acme.DNS01, acme.TLSSNI01})

	return client, nil
}
